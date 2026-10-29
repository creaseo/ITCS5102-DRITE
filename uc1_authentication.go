package drite

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var fmtInvalidJSON = errors.New("expected one JSON object")
var agentIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type connectionTicket struct {
	Role    string
	AgentID string
	Expires time.Time
}

func (s *Server) authenticate(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	actual := sha256.Sum256([]byte(token))
	expected := sha256.Sum256([]byte(s.config.AuthToken))
	return subtle.ConstantTimeCompare(actual[:], expected[:]) == 1
}
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authenticate(r) {
		return true
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	apiError(w, 401, "valid bearer token required")
	return false
}

// HandleAuth exchanges the shared secret for a one-use, role-bound 60-second ticket.
func (s *Server) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") || !s.requireAuth(w, r) {
		return
	}
	var req struct {
		Role    string `json:"role"`
		AgentID string `json:"agent_id"`
	}
	if decodeJSON(w, r, &req) != nil || (req.Role != "agent" && req.Role != "dashboard") || (req.Role == "agent" && !agentIDPattern.MatchString(req.AgentID)) || (req.Role == "dashboard" && req.AgentID != "") {
		apiError(w, 400, "role must be agent (with agent_id) or dashboard")
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		apiError(w, 500, "could not generate ticket")
		return
	}
	ticket := hex.EncodeToString(b)
	expires := time.Now().UTC().Add(time.Minute)
	s.ticketsMu.Lock()
	for k, v := range s.tickets {
		if !v.Expires.After(time.Now()) {
			delete(s.tickets, k)
		}
	}
	if len(s.tickets) >= 10000 {
		s.ticketsMu.Unlock()
		apiError(w, 503, "ticket capacity reached")
		return
	}
	s.tickets[ticket] = connectionTicket{req.Role, req.AgentID, expires}
	s.ticketsMu.Unlock()
	writeJSON(w, 201, map[string]any{"ticket": ticket, "expires_at": expires})
}
func (s *Server) authorizeSocket(r *http.Request, role, agentID string) bool {
	if s.authenticate(r) {
		return true
	}
	ticket := r.URL.Query().Get("ticket")
	s.ticketsMu.Lock()
	defer s.ticketsMu.Unlock()
	t, ok := s.tickets[ticket]
	if !ok || !t.Expires.After(time.Now()) || t.Role != role || t.AgentID != agentID {
		return false
	}
	delete(s.tickets, ticket)
	return true
}
func (s *Server) HandleAgentWS(w http.ResponseWriter, r *http.Request) { s.connect(w, r, "agent") }
func (s *Server) HandleDashboardWS(w http.ResponseWriter, r *http.Request) {
	s.connect(w, r, "dashboard")
}
func (s *Server) connect(w http.ResponseWriter, r *http.Request, role string) {
	if !method(w, r, "GET") {
		return
	}
	id := ""
	if role == "agent" {
		id = r.URL.Query().Get("agent_id")
		if !agentIDPattern.MatchString(id) {
			apiError(w, 400, "valid agent_id query parameter required")
			return
		}
	}
	if !s.authorizeSocket(r, role, id) {
		apiError(w, 401, "valid bearer token or connection ticket required")
		return
	}
	// Serialize upgrade/admission with Close so no goroutine can start after shutdown.
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		apiError(w, 503, "server shutting down")
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{server: s, conn: conn, agentID: id, dashboard: role == "dashboard", send: make(chan []byte, 256)}
	select {
	case s.hub.register <- c:
	case <-s.ctx.Done():
		conn.Close()
		return
	}
	s.wg.Add(2)
	go func() { defer s.wg.Done(); c.readPump() }()
	go func() { defer s.wg.Done(); c.writePump() }()
}
