package drite

import "time"

func (s *Server) cpuThreshold() float64 {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.threshold
}

func (s *Server) alertCooldown() time.Duration {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.cooldown
}

func evaluateCPU(m MetricMessage, threshold float64) *AlertMessage {
	if m.CPUUsage <= threshold {
		return nil
	}
	return &AlertMessage{Level: "warning", Kind: "cpu_threshold", AgentID: m.AgentID, Value: m.CPUUsage, Threshold: threshold, OccurredAt: m.Timestamp, Message: "CPU usage exceeded the configured threshold"}
}

// One processing goroutine orders aggregation, alerts and telemetry. Alerts enter
// the broadcast queue before their triggering metric; historical events stay FIFO.
func (s *Server) processMetrics() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case metric := <-s.ingest:
			s.aggregator.Add(metric)
			now := time.Now().UTC()
			if alert := evaluateCPU(metric, s.cpuThreshold()); alert != nil {
				cooldown := s.alertCooldown()
				s.configMu.Lock()
				lastTime, exists := s.lastAlerts[metric.AgentID]
				shouldFire := !exists || cooldown <= 0 || now.Sub(lastTime) >= cooldown
				if shouldFire {
					s.lastAlerts[metric.AgentID] = now
				}
				s.configMu.Unlock()

				if shouldFire {
					s.aggregator.addIncident(*alert)
					if !s.publish(BroadcastEnvelope{Type: "alert", Alert: alert, ServerTime: now}) {
						return
					}
				}
			}
			avg := s.aggregator.Averages(now)
			if !s.publish(BroadcastEnvelope{Type: "metric", Metric: &metric, Averages: &avg, ServerTime: now}) {
				return
			}
		}
	}
}
