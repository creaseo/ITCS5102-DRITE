package drite

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Server owns its goroutines and in-memory state. Close must be called by its owner.
type Server struct {
	config     Config
	configMu   sync.RWMutex
	threshold  float64
	upgrader   websocket.Upgrader
	aggregator *RollingAggregator
	hub        *Hub
	ingest     chan MetricMessage
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	lifecycle  sync.Mutex
	closed     bool
	closeOnce  sync.Once
	ticketsMu  sync.Mutex
	tickets    map[string]connectionTicket
}

func NewServer(cfg Config) *Server {
	cfg = defaultConfig(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{config: cfg, threshold: cfg.CPUThreshold, ctx: ctx, cancel: cancel,
		ingest: make(chan MetricMessage, 256), aggregator: NewRollingAggregator(5 * time.Minute), tickets: make(map[string]connectionTicket)}
	// Gorilla's default origin policy permits missing Origin or the request's own host.
	s.upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	s.hub = newHub(s)
	s.wg.Add(3)
	go func() { defer s.wg.Done(); s.hub.Run() }()
	go func() { defer s.wg.Done(); s.processMetrics() }()
	go func() { defer s.wg.Done(); s.maintainState() }()
	return s
}
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.lifecycle.Lock()
		s.closed = true
		s.cancel()
		s.lifecycle.Unlock()
		s.wg.Wait()
	})
}
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/agent", s.HandleAgentWS)
	mux.HandleFunc("/ws/dashboard", s.HandleDashboardWS)
	mux.HandleFunc("/api/settings", s.HandleSettings)
	mux.HandleFunc("/api/metrics/averages", s.HandleAverages)
	mux.HandleFunc("/api/v1/auth", s.HandleAuth)
	mux.HandleFunc("/api/v1/snapshot", s.HandleSnapshot)
	return mux
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method == allowed {
		return true
	}
	w.Header().Set("Allow", allowed)
	apiError(w, 405, "method not allowed")
	return false
}
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmtInvalidJSON
	}
	return nil
}
