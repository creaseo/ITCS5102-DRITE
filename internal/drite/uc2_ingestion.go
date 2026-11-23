package drite

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"github.com/gorilla/websocket"
)

func parseMetric(data []byte, agentID string, now time.Time) (MetricMessage, error) {
	// Pointers distinguish a missing field from a legitimate zero measurement.
	var input struct {
		AgentID   string    `json:"agent_id"`
		CPU       *float64  `json:"cpu_usage"`
		Memory    *float64  `json:"memory_usage"`
		Network   *float64  `json:"network_speed"`
		Timestamp time.Time `json:"timestamp"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return MetricMessage{}, err
	}
	if d.Decode(new(any)) != io.EOF {
		return MetricMessage{}, fmtInvalidJSON
	}
	if input.CPU == nil || input.Memory == nil || input.Network == nil {
		return MetricMessage{}, errors.New("cpu_usage, memory_usage and network_speed are required")
	}
	if input.AgentID != "" && input.AgentID != agentID {
		return MetricMessage{}, errors.New("agent_id does not match connection")
	}
	for _, v := range []float64{*input.CPU, *input.Memory, *input.Network} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return MetricMessage{}, errors.New("metrics must be finite and nonnegative")
		}
	}
	if *input.CPU > 100 || *input.Memory > 100 {
		return MetricMessage{}, errors.New("CPU and memory must be percentages in [0,100]")
	}
	return MetricMessage{agentID, *input.CPU, *input.Memory, *input.Network, now.UTC()}, nil
}
func (c *Client) readPump() {
	defer c.disconnect()
	c.prepareHeartbeat()
	for {
		kind, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if c.dashboard {
			continue
		} // Dashboard reads still process protocol Pong frames.
		if kind != websocket.TextMessage {
			c.policyClose("expected a text JSON frame")
			return
		}
		metric, err := parseMetric(data, c.agentID, time.Now())
		if err != nil {
			c.policyClose("invalid metric: " + err.Error())
			return
		}
		select {
		case c.server.ingest <- metric:
		case <-c.server.ctx.Done():
			return
		}
	}
}
