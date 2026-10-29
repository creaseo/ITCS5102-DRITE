package drite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T, cfg Config) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(cfg)
	h := httptest.NewServer(s.Routes())
	t.Cleanup(func() { s.Close(); h.Close() })
	return s, h
}
func request(t *testing.T, h *httptest.Server, method, path, body string, auth bool) (int, []byte) {
	t.Helper()
	r, err := http.NewRequest(method, h.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if auth {
		r.Header.Set("Authorization", "Bearer drite-demo-token")
	}
	response, err := h.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}
func socket(t *testing.T, h *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	headers := http.Header{"Authorization": []string{"Bearer drite-demo-token"}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.URL, "http")+path, headers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
func event(t *testing.T, c *websocket.Conn) BroadcastEnvelope {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var e BroadcastEnvelope
	if err := c.ReadJSON(&e); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestUC1AuthenticationAndOrigin(t *testing.T) {
	_, h := fixture(t, Config{})
	for _, path := range []string{"/api/settings", "/api/metrics/averages", "/api/v1/snapshot"} {
		code, _ := request(t, h, "GET", path, "", false)
		if code != 401 {
			t.Fatalf("%s: %d", path, code)
		}
	}
	url := "ws" + strings.TrimPrefix(h.URL, "http") + "/ws/dashboard"
	for _, tc := range []struct {
		headers http.Header
		status  int
	}{
		{nil, 401},
		{http.Header{"Authorization": []string{"Bearer wrong"}}, 401},
		{http.Header{"Authorization": []string{"Bearer drite-demo-token"}, "Origin": []string{"https://untrusted.example"}}, 403},
	} {
		c, r, err := websocket.DefaultDialer.Dial(url, tc.headers)
		if c != nil {
			c.Close()
		}
		if err == nil || r == nil || r.StatusCode != tc.status {
			t.Fatalf("expected %d, response %v, err %v", tc.status, r, err)
		}
		r.Body.Close()
	}
}
func TestUC1SingleUseTicket(t *testing.T) {
	s, h := fixture(t, Config{})
	code, data := request(t, h, "POST", "/api/v1/auth", `{"role":"agent","agent_id":"node-a"}`, true)
	if code != 201 {
		t.Fatalf("%d %s", code, data)
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(data, &ticket); err != nil {
		t.Fatal(err)
	}
	base := "ws" + strings.TrimPrefix(h.URL, "http")
	c, r, err := websocket.DefaultDialer.Dial(base+"/ws/dashboard?ticket="+ticket.Ticket, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || r.StatusCode != 401 {
		t.Fatal("role binding failed")
	}
	r.Body.Close()
	path := base + "/ws/agent?agent_id=node-a&ticket=" + ticket.Ticket
	c, _, err = websocket.DefaultDialer.Dial(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	c, r, err = websocket.DefaultDialer.Dial(path, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || r.StatusCode != 401 {
		t.Fatal("ticket replay succeeded")
	}
	r.Body.Close()
	s.ticketsMu.Lock()
	s.tickets["expired"] = connectionTicket{Role: "dashboard", Expires: time.Now().Add(-time.Second)}
	s.ticketsMu.Unlock()
	req := httptest.NewRequest("GET", "/ws/dashboard?ticket=expired", nil)
	if s.authorizeSocket(req, "dashboard", "") {
		t.Fatal("expired ticket accepted")
	}
}
func TestUC2Validation(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"cpu_usage":-1,"memory_usage":4,"network_speed":5}`, `{"cpu_usage":101,"memory_usage":4,"network_speed":5}`, `{"cpu_usage":4,"memory_usage":101,"network_speed":5}`, `{"cpu_usage":4,"memory_usage":4,"network_speed":-1}`, `{"cpu_usage":4,"memory_usage":4,"network_speed":5,"extra":1}`, `{"cpu_usage":4,"memory_usage":4,"network_speed":5,"agent_id":"spoof"}`, `{"cpu_usage":4,"memory_usage":4,"network_speed":5} {}`, `{"cpu_usage":4,"memory_usage":4,"network_speed":1e309}`} {
		if _, err := parseMetric([]byte(body), "node", time.Now()); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	now := time.Now().UTC()
	m, err := parseMetric([]byte(`{"cpu_usage":0,"memory_usage":0,"network_speed":0,"timestamp":"2099-01-01T00:00:00Z"}`), "node", now)
	if err != nil || !m.Timestamp.Equal(now) || m.AgentID != "node" {
		t.Fatalf("%+v %v", m, err)
	}
}
func TestUC2InvalidAgentDoesNotAffectOthers(t *testing.T) {
	_, h := fixture(t, Config{})
	bad := socket(t, h, "/ws/agent?agent_id=bad")
	good := socket(t, h, "/ws/agent?agent_id=good")
	dash := socket(t, h, "/ws/dashboard")
	if err := bad.WriteMessage(websocket.TextMessage, []byte(`{"cpu_usage":999}`)); err != nil {
		t.Fatal(err)
	}
	bad.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err := bad.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("expected policy close: %v", err)
	}
	if err = good.WriteJSON(MetricMessage{CPUUsage: 20, MemoryUsage: 40, NetworkSpeed: 100}); err != nil {
		t.Fatal(err)
	}
	if event(t, dash).Metric.AgentID != "good" {
		t.Fatal("healthy agent interrupted")
	}
}
func TestUC3AndUC6BroadcastAlertOrder(t *testing.T) {
	_, h := fixture(t, Config{})
	a := socket(t, h, "/ws/agent?agent_id=node")
	d1 := socket(t, h, "/ws/dashboard")
	d2 := socket(t, h, "/ws/dashboard")
	if err := a.WriteJSON(MetricMessage{CPUUsage: 95, MemoryUsage: 45, NetworkSpeed: 1000}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []*websocket.Conn{d1, d2} {
		alert := event(t, d)
		metric := event(t, d)
		if alert.Type != "alert" || alert.Alert.Kind != "cpu_threshold" || metric.Type != "metric" || metric.Averages.OneMinute.Count != 1 {
			t.Fatalf("bad events: %+v %+v", alert, metric)
		}
	}
	code, data := request(t, h, "GET", "/api/v1/snapshot", "", true)
	var snap Snapshot
	if json.Unmarshal(data, &snap) != nil || code != 200 || len(snap.Incidents) != 1 || len(snap.Metrics) != 1 || snap.Agents["node"].OneMinute.CPUUsage != 95 {
		t.Fatalf("snapshot %s", data)
	}
}
func TestUC4WindowBoundariesAndIsolation(t *testing.T) {
	now := time.Now().UTC()
	a := NewRollingAggregator(5 * time.Minute)
	// Deliberately out of order, including an expired sample and a future clock.
	for _, m := range []MetricMessage{
		{AgentID: "a", CPUUsage: 20, Timestamp: now.Add(-2 * time.Minute)},
		{AgentID: "b", CPUUsage: 40, Timestamp: now.Add(-time.Minute)},
		{AgentID: "a", CPUUsage: 100, Timestamp: now.Add(-6 * time.Minute)},
		{AgentID: "a", CPUUsage: 60, Timestamp: now},
		{AgentID: "a", CPUUsage: 100, Timestamp: now.Add(time.Minute)},
	} {
		a.Add(m)
	}
	avg := a.Averages(now)
	if avg.OneMinute.Count != 2 || avg.OneMinute.CPUUsage != 50 || avg.FiveMinute.Count != 3 || avg.FiveMinute.CPUUsage != 40 {
		t.Fatalf("%+v", avg)
	}
	snap := a.Snapshot(now)
	if snap.Agents["a"].FiveMinute.CPUUsage != 40 {
		t.Fatal("per-agent average")
	}
	snap.Metrics[0].CPUUsage = 999
	if a.Averages(now).FiveMinute.CPUUsage != 40 {
		t.Fatal("snapshot aliases store")
	}
	if a.Averages(now.Add(10*time.Minute)).FiveMinute.Count != 0 {
		t.Fatal("stale values remain")
	}
}
func TestUC4CapacityAndPruning(t *testing.T) {
	a := NewRollingAggregator(5 * time.Minute)
	now := time.Now()
	for i := 0; i < maxSamples+3; i++ {
		a.Add(MetricMessage{AgentID: "a", CPUUsage: 1, Timestamp: now})
	}
	if len(a.samples) != maxSamples || a.dropped != 3 {
		t.Fatal("sample bound")
	}
	a.mu.Lock()
	a.pruneLocked(now.Add(6 * time.Minute))
	a.mu.Unlock()
	if len(a.samples) != 0 {
		t.Fatal("pruning failed")
	}
	for i := 0; i < maxIncidents+1; i++ {
		a.addIncident(AlertMessage{Value: float64(i)})
	}
	if len(a.incidents) != maxIncidents || a.incidents[0].Value != 1 {
		t.Fatal("incident bound")
	}
}
func TestUC5HeartbeatExpiry(t *testing.T) {
	_, h := fixture(t, Config{PingInterval: 15 * time.Millisecond, PongWait: 80 * time.Millisecond})
	c := socket(t, h, "/ws/dashboard")
	c.SetPingHandler(func(string) error { return nil })
	c.SetReadDeadline(time.Now().Add(time.Second))
	start := time.Now()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
	if time.Since(start) > 700*time.Millisecond {
		t.Fatal("server did not evict missing pong")
	}
}
func TestUC5ResponsiveConnectionAndShutdown(t *testing.T) {
	s, h := fixture(t, Config{PingInterval: 20 * time.Millisecond, PongWait: 150 * time.Millisecond})
	c := socket(t, h, "/ws/dashboard")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
		t.Fatal("responsive socket closed")
	case <-time.After(350 * time.Millisecond):
	}
	s.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown left connection open")
	}
	s.Close()
}
func TestUC7Settings(t *testing.T) {
	_, h := fixture(t, Config{})
	for _, body := range []string{`null`, `{}`, `{"cpu_threshold":null}`, `{"cpu_threshold":-1}`, `{"cpu_threshold":101}`, `{"cpu_threshold":80,"typo":1}`, `{"cpu_threshold":80} {}`, strings.Repeat("x", 5000)} {
		code, _ := request(t, h, "POST", "/api/settings", body, true)
		if code != 400 {
			t.Fatalf("accepted %s", body)
		}
	}
	code, data := request(t, h, "POST", "/api/settings", `{"cpu_threshold":0}`, true)
	if code != 200 || !bytes.Contains(data, []byte(`"cpu_threshold":0`)) {
		t.Fatalf("%d %s", code, data)
	}
	code, _ = request(t, h, "DELETE", "/api/settings", "", true)
	if code != 405 {
		t.Fatal(code)
	}
	if evaluateCPU(MetricMessage{CPUUsage: 90}, 90) != nil {
		t.Fatal("threshold must be strictly exceeded")
	}
}
func TestConcurrentIngestionAndSnapshots(t *testing.T) {
	_, h := fixture(t, Config{})
	const agents = 12
	const samples = 15
	var wg sync.WaitGroup
	connections := make([]*websocket.Conn, agents)
	for i := range connections {
		connections[i] = socket(t, h, fmt.Sprintf("/ws/agent?agent_id=node-%d", i))
	}
	errors := make(chan error, agents)
	for _, conn := range connections {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for j := 0; j < samples; j++ {
				if err := c.WriteJSON(MetricMessage{CPUUsage: 50, MemoryUsage: 30, NetworkSpeed: 20}); err != nil {
					errors <- err
					return
				}
			}
		}(conn)
	}
	for i := 0; i < 10; i++ {
		request(t, h, "POST", "/api/settings", `{"cpu_threshold":80}`, true)
		request(t, h, "GET", "/api/v1/snapshot", "", true)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, data := request(t, h, "GET", "/api/metrics/averages", "", true)
		var avg RollingAverages
		json.Unmarshal(data, &avg)
		if avg.OneMinute.Count == agents*samples {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lost metrics: %+v", avg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func TestConfigValidation(t *testing.T) {
	t.Setenv("PORT", "8081")
	t.Setenv("DRITE_CPU_THRESHOLD", "75")
	cfg, addr, err := LoadConfig()
	if err != nil || addr != ":8081" || cfg.CPUThreshold != 75 {
		t.Fatal(cfg, addr, err)
	}
	for _, v := range []string{"NaN", "Inf", "0", "101", "oops"} {
		t.Setenv("DRITE_CPU_THRESHOLD", v)
		if _, _, err = LoadConfig(); err == nil {
			t.Fatal("accepted", v)
		}
	}
	t.Setenv("DRITE_CPU_THRESHOLD", "90")
	t.Setenv("PORT", "70000")
	if _, _, err = LoadConfig(); err == nil {
		t.Fatal("bad port accepted")
	}
}
func BenchmarkParseMetric(b *testing.B) {
	data := []byte(`{"cpu_usage":50,"memory_usage":30,"network_speed":1000}`)
	now := time.Now()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := parseMetric(data, "node", now); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRollingAverages(b *testing.B) {
	a := NewRollingAggregator(5 * time.Minute)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		a.Add(MetricMessage{CPUUsage: 50, Timestamp: now})
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			a.Averages(now)
		}
	})
}

func TestUC3SlowConsumerEviction(t *testing.T) {
	// Use real sockets but an isolated hub with a deliberately full output queue.
	// This avoids relying on operating-system buffer sizes to simulate a slow reader.
	_, h := fixture(t, Config{})
	slowConn := socket(t, h, "/ws/dashboard")
	healthyConn := socket(t, h, "/ws/dashboard")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	isolated := &Server{ctx: ctx}
	hub := newHub(isolated)
	slow := &Client{conn: slowConn, dashboard: true, send: make(chan []byte, 1)}
	healthy := &Client{conn: healthyConn, dashboard: true, send: make(chan []byte, 1)}
	slow.send <- []byte("old")
	hub.clients[slow] = struct{}{}
	hub.clients[healthy] = struct{}{}
	done := make(chan struct{})
	go func() { defer close(done); hub.Run() }()
	hub.broadcast <- []byte("new")
	select {
	case data := <-healthy.send:
		if string(data) != "new" {
			t.Fatal("wrong event")
		}
	case <-time.After(time.Second):
		t.Fatal("slow consumer blocked healthy consumer")
	}
	<-slow.send // The old queued event can still be drained after eviction.
	select {
	case _, open := <-slow.send:
		if open {
			t.Fatal("slow client queue was not closed")
		}
	case <-time.After(time.Second):
		t.Fatal("slow client was not evicted")
	}
	cancel()
	<-done
	if _, ok := hub.clients[slow]; ok {
		t.Fatal("slow client retained")
	}
}

func TestUC4LargeFiniteNetworkAverage(t *testing.T) {
	now := time.Now()
	values := []MetricMessage{{NetworkSpeed: 1e308, Timestamp: now}, {NetworkSpeed: 1e308, Timestamp: now}}
	got := average(values, now, time.Minute, "")
	if got.NetworkSpeed != 1e308 {
		t.Fatalf("average overflowed: %v", got.NetworkSpeed)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatal(err)
	}
}

func TestUC2OversizedMessage(t *testing.T) {
	_, h := fixture(t, Config{ReadLimitByte: 128})
	c := socket(t, h, "/ws/agent?agent_id=node")
	if err := c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 256))); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err := c.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("expected size rejection: %v", err)
	}
}

func TestUC6RuntimeThresholdChanges(t *testing.T) {
	_, h := fixture(t, Config{})
	agent := socket(t, h, "/ws/agent?agent_id=node")
	dashboard := socket(t, h, "/ws/dashboard")
	sample := MetricMessage{CPUUsage: 85, MemoryUsage: 40, NetworkSpeed: 50}
	if err := agent.WriteJSON(sample); err != nil {
		t.Fatal(err)
	}
	if e := event(t, dashboard); e.Type != "metric" {
		t.Fatal("unexpected alert before threshold change")
	}
	code, _ := request(t, h, "POST", "/api/settings", `{"cpu_threshold":80}`, true)
	if code != 200 {
		t.Fatal(code)
	}
	if err := agent.WriteJSON(sample); err != nil {
		t.Fatal(err)
	}
	e := event(t, dashboard)
	if e.Type != "alert" || e.Alert.Threshold != 80 {
		t.Fatalf("threshold update not applied: %+v", e)
	}
	if event(t, dashboard).Type != "metric" {
		t.Fatal("missing metric")
	}
}
