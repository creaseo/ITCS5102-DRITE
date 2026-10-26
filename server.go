package drite

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultAuthToken = "drite-demo-token"
)

type Config struct {
	AuthToken     string
	CPUThreshold  float64
	PingInterval  time.Duration
	PongWait      time.Duration
	WriteWait     time.Duration
	ReadLimitByte int64
}

type Server struct {
	upgrader   websocket.Upgrader
	hub        *Hub
	aggregator *RollingAggregator

	configMu sync.RWMutex
	config   Config
}

type MetricMessage struct {
	AgentID      string    `json:"agent_id"`
	CPUUsage     float64   `json:"cpu_usage"`
	MemoryUsage  float64   `json:"memory_usage"`
	NetworkSpeed float64   `json:"network_speed"`
	Timestamp    time.Time `json:"timestamp"`
}

type MetricAverage struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	NetworkSpeed float64 `json:"network_speed"`
	Count        int     `json:"count"`
}

type RollingAverages struct {
	OneMinute  MetricAverage `json:"one_minute"`
	FiveMinute MetricAverage `json:"five_minute"`
}

type AlertMessage struct {
	Level      string    `json:"level"`
	Kind       string    `json:"kind"`
	AgentID    string    `json:"agent_id"`
	Value      float64   `json:"value"`
	Threshold  float64   `json:"threshold"`
	OccurredAt time.Time `json:"occurred_at"`
	Message    string    `json:"message"`
}

type BroadcastEnvelope struct {
	Type         string           `json:"type"`
	Metric       *MetricMessage   `json:"metric,omitempty"`
	Averages     *RollingAverages `json:"averages,omitempty"`
	Alert        *AlertMessage    `json:"alert,omitempty"`
	ServerTime   time.Time        `json:"server_time"`
	HeartbeatUTC time.Time        `json:"heartbeat_utc"`
}

type settingsResponse struct {
	AuthRequired bool    `json:"auth_required"`
	CPUThreshold float64 `json:"cpu_threshold"`
}

type settingsRequest struct {
	CPUThreshold float64 `json:"cpu_threshold"`
}

type Client struct {
	server *Server
	conn   *websocket.Conn
	send   chan []byte
}

type Hub struct {
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

func NewServer(cfg Config) *Server {
	if cfg.AuthToken == "" {
		cfg.AuthToken = defaultAuthToken
	}
	if cfg.CPUThreshold <= 0 {
		cfg.CPUThreshold = 90
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 20 * time.Second
	}
	if cfg.PongWait <= 0 {
		cfg.PongWait = 45 * time.Second
	}
	if cfg.WriteWait <= 0 {
		cfg.WriteWait = 10 * time.Second
	}
	if cfg.ReadLimitByte <= 0 {
		cfg.ReadLimitByte = 1 << 20
	}

	h := &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
	}
	go h.Run()

	return &Server{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
		hub:        h,
		aggregator: NewRollingAggregator(5 * time.Minute),
		config:     cfg,
	}
}

func NewRollingAggregator(window time.Duration) *RollingAggregator {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &RollingAggregator{window: window}
}

type RollingAggregator struct {
	mu      sync.RWMutex
	window  time.Duration
	samples []MetricMessage
}

func (a *RollingAggregator) Add(sample MetricMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.samples = append(a.samples, sample)
	a.prune(sample.Timestamp)
}

func (a *RollingAggregator) Averages(now time.Time) RollingAverages {
	a.mu.RLock()
	defer a.mu.RUnlock()
	oneStart := now.Add(-1 * time.Minute)
	fiveStart := now.Add(-5 * time.Minute)

	var one, five MetricAverage
	for _, s := range a.samples {
		if s.Timestamp.Before(fiveStart) {
			continue
		}
		five.CPUUsage += s.CPUUsage
		five.MemoryUsage += s.MemoryUsage
		five.NetworkSpeed += s.NetworkSpeed
		five.Count++
		if s.Timestamp.Before(oneStart) {
			continue
		}
		one.CPUUsage += s.CPUUsage
		one.MemoryUsage += s.MemoryUsage
		one.NetworkSpeed += s.NetworkSpeed
		one.Count++
	}

	if one.Count > 0 {
		one.CPUUsage /= float64(one.Count)
		one.MemoryUsage /= float64(one.Count)
		one.NetworkSpeed /= float64(one.Count)
	}
	if five.Count > 0 {
		five.CPUUsage /= float64(five.Count)
		five.MemoryUsage /= float64(five.Count)
		five.NetworkSpeed /= float64(five.Count)
	}

	return RollingAverages{OneMinute: one, FiveMinute: five}
}

func (a *RollingAggregator) prune(now time.Time) {
	cutoff := now.Add(-a.window)
	idx := 0
	for idx < len(a.samples) && a.samples[idx].Timestamp.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		a.samples = append([]MetricMessage(nil), a.samples[idx:]...)
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/agent", s.HandleAgentWS)
	mux.HandleFunc("/ws/dashboard", s.HandleDashboardWS)
	mux.HandleFunc("/api/settings", s.HandleSettings)
	mux.HandleFunc("/api/metrics/averages", s.HandleAverages)
	return mux
}

func (s *Server) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = "unknown-agent"
	}
	go s.readAgentStream(conn, agentID)
}