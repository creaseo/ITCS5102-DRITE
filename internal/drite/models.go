package drite

import "time"

// MetricMessage uses percentages for CPU and memory, and bytes/second for network.
// Timestamp is assigned by the server so agent clocks cannot distort windows.
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
	Type       string           `json:"type"`
	Metric     *MetricMessage   `json:"metric,omitempty"`
	Averages   *RollingAverages `json:"averages,omitempty"`
	Alert      *AlertMessage    `json:"alert,omitempty"`
	ServerTime time.Time        `json:"server_time"`
}
