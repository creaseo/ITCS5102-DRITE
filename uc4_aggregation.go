package drite

import (
	"sync"
	"time"
)

const maxSamples = 10000
const maxIncidents = 1000

// RollingAggregator keeps bounded raw history. Under overload, averages describe
// the newest retained samples, not a claim of complete five-minute coverage.
type RollingAggregator struct {
	mu        sync.RWMutex
	window    time.Duration
	samples   []MetricMessage
	incidents []AlertMessage
	dropped   uint64
}

func NewRollingAggregator(window time.Duration) *RollingAggregator {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &RollingAggregator{window: window, samples: make([]MetricMessage, 0), incidents: make([]AlertMessage, 0)}
}
func (a *RollingAggregator) Add(m MetricMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(time.Now())
	if len(a.samples) == maxSamples {
		copy(a.samples, a.samples[1:])
		a.samples = a.samples[:maxSamples-1]
		a.dropped++
	}
	a.samples = append(a.samples, m)
}
func (a *RollingAggregator) pruneLocked(now time.Time) {
	cutoff := now.Add(-a.window)
	kept := a.samples[:0]
	for _, m := range a.samples {
		if !m.Timestamp.Before(cutoff) {
			kept = append(kept, m)
		}
	}
	// Clear removed references so disconnected agent names can be reclaimed.
	for i := len(kept); i < len(a.samples); i++ {
		a.samples[i] = MetricMessage{}
	}
	a.samples = kept
}
func average(samples []MetricMessage, now time.Time, window time.Duration, agent string) MetricAverage {
	var result MetricAverage
	for _, m := range samples {
		if (agent != "" && m.AgentID != agent) || m.Timestamp.Before(now.Add(-window)) || m.Timestamp.After(now) {
			continue
		}
		result.Count++
		// An incremental mean avoids overflowing the sum of large finite values.
		n := float64(result.Count)
		result.CPUUsage += (m.CPUUsage - result.CPUUsage) / n
		result.MemoryUsage += (m.MemoryUsage - result.MemoryUsage) / n
		result.NetworkSpeed += (m.NetworkSpeed - result.NetworkSpeed) / n
	}
	return result
}
func averages(samples []MetricMessage, now time.Time, agent string) RollingAverages {
	return RollingAverages{average(samples, now, time.Minute, agent), average(samples, now, 5*time.Minute, agent)}
}
func (a *RollingAggregator) Averages(now time.Time) RollingAverages {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return averages(a.samples, now, "")
}
func (a *RollingAggregator) addIncident(alert AlertMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.incidents) == maxIncidents {
		copy(a.incidents, a.incidents[1:])
		a.incidents = a.incidents[:maxIncidents-1]
	}
	a.incidents = append(a.incidents, alert)
}
func (s *Server) maintainState() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.aggregator.mu.Lock()
			s.aggregator.pruneLocked(now)
			s.aggregator.mu.Unlock()
			s.ticketsMu.Lock()
			for token, t := range s.tickets {
				if !t.Expires.After(now) {
					delete(s.tickets, token)
				}
			}
			s.ticketsMu.Unlock()
		}
	}
}
