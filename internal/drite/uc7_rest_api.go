package drite

import (
	"math"
	"net/http"
	"time"
)

func (s *Server) HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		w.Header().Set("Allow", "GET, POST")
		apiError(w, 405, "method not allowed")
		return
	}
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method == "POST" {
		var body struct {
			CPUThreshold     *float64 `json:"cpu_threshold"`
			AlertCooldownSec *float64 `json:"alert_cooldown_sec"`
		}
		if decodeJSON(w, r, &body) != nil {
			apiError(w, 400, "invalid body")
			return
		}
		if body.CPUThreshold == nil && body.AlertCooldownSec == nil {
			apiError(w, 400, "at least one of cpu_threshold or alert_cooldown_sec is required")
			return
		}
		if body.CPUThreshold != nil && (math.IsNaN(*body.CPUThreshold) || math.IsInf(*body.CPUThreshold, 0) || *body.CPUThreshold < 0 || *body.CPUThreshold > 100) {
			apiError(w, 400, "cpu_threshold must be a number in [0,100]")
			return
		}
		if body.AlertCooldownSec != nil && (math.IsNaN(*body.AlertCooldownSec) || math.IsInf(*body.AlertCooldownSec, 0) || *body.AlertCooldownSec < 0 || *body.AlertCooldownSec > 3600) {
			apiError(w, 400, "alert_cooldown_sec must be a number in [0,3600]")
			return
		}
		s.configMu.Lock()
		if body.CPUThreshold != nil {
			s.threshold = *body.CPUThreshold
		}
		if body.AlertCooldownSec != nil {
			s.cooldown = time.Duration(*body.AlertCooldownSec * float64(time.Second))
		}
		s.configMu.Unlock()
	}
	writeJSON(w, 200, map[string]any{
		"auth_required":      true,
		"cpu_threshold":      s.cpuThreshold(),
		"alert_cooldown_sec": s.alertCooldown().Seconds(),
	})
}
func (s *Server) HandleAverages(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") || !s.requireAuth(w, r) {
		return
	}
	now := time.Now().UTC()
	agent := r.URL.Query().Get("agent_id")
	s.aggregator.mu.RLock()
	avg := averages(s.aggregator.samples, now, agent)
	s.aggregator.mu.RUnlock()
	writeJSON(w, 200, avg)
}

type Snapshot struct {
	ServerTime      time.Time                  `json:"server_time"`
	Averages        RollingAverages            `json:"averages"`
	Agents          map[string]RollingAverages `json:"agents"`
	Metrics         []MetricMessage            `json:"metrics"`
	Incidents       []AlertMessage             `json:"incidents"`
	CapacityDropped uint64                     `json:"capacity_dropped"`
}

func (a *RollingAggregator) Snapshot(now time.Time) Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := Snapshot{ServerTime: now, Averages: averages(a.samples, now, ""), Agents: make(map[string]RollingAverages), Metrics: make([]MetricMessage, 0), Incidents: append([]AlertMessage{}, a.incidents...), CapacityDropped: a.dropped}
	grouped := make(map[string][]MetricMessage)
	for _, m := range a.samples {
		if m.Timestamp.Before(now.Add(-a.window)) || m.Timestamp.After(now) {
			continue
		}
		result.Metrics = append(result.Metrics, m)
		grouped[m.AgentID] = append(grouped[m.AgentID], m)
	}
	for id, samples := range grouped {
		result.Agents[id] = averages(samples, now, "")
	}
	return result
}
func (s *Server) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") || !s.requireAuth(w, r) {
		return
	}
	writeJSON(w, 200, s.aggregator.Snapshot(time.Now().UTC()))
}
