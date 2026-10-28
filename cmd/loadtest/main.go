// A repeatable synthetic workload. It reports measured observations, not targets.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	drite "github.com/creaseo/DRITE"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	base := flag.String("url", "http://localhost:8080", "server HTTP URL")
	agents := flag.Int("agents", 10, "number of synthetic agents")
	dashboards := flag.Int("dashboards", 2, "number of dashboard subscribers")
	duration := flag.Duration("duration", 10*time.Second, "measurement duration")
	interval := flag.Duration("interval", 100*time.Millisecond, "send interval per agent")
	flag.Parse()
	if *agents < 1 || *dashboards < 1 || *duration <= 0 || *interval <= 0 {
		return fmt.Errorf("counts, duration and interval must be positive")
	}
	u, err := url.Parse(*base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("url must be an http or https server URL")
	}
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	token := os.Getenv("DRITE_AUTH_TOKEN")
	if token == "" {
		token = "drite-demo-token"
	}
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	var conns []*websocket.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	dial := func(path string) (*websocket.Conn, error) {
		target := *u
		p, err := url.Parse(path)
		if err != nil {
			return nil, err
		}
		target.Path = p.Path
		target.RawQuery = p.RawQuery
		c, _, err := websocket.DefaultDialer.Dial(target.String(), headers)
		if err == nil {
			conns = append(conns, c)
		}
		return c, err
	}
	var wg sync.WaitGroup
	var sent, received, failures atomic.Int64
	var stopping atomic.Bool
	var latencyMu sync.Mutex
	latencies := make([]float64, 0, 100000)
	for i := 0; i < *dashboards; i++ {
		c, err := dial("/ws/dashboard")
		if err != nil {
			return fmt.Errorf("dashboard %d: %w", i, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var e drite.BroadcastEnvelope
				if err := c.ReadJSON(&e); err != nil {
					if !stopping.Load() {
						failures.Add(1)
					}
					return
				}
				if e.Type == "metric" {
					received.Add(1)
					ms := float64(time.Since(e.ServerTime).Microseconds()) / 1000
					latencyMu.Lock()
					if len(latencies) < cap(latencies) {
						latencies = append(latencies, ms)
					}
					latencyMu.Unlock()
				}
			}
		}()
	}
	agentsConns := make([]*websocket.Conn, 0, *agents)
	for i := 0; i < *agents; i++ {
		c, err := dial(fmt.Sprintf("/ws/agent?agent_id=load-%d", i))
		if err != nil {
			stopping.Store(true)
			return fmt.Errorf("agent %d: %w", i, err)
		}
		agentsConns = append(agentsConns, c)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					if !stopping.Load() {
						failures.Add(1)
					}
					return
				}
			}
		}()
	}
	start := time.Now()
	stop := make(chan struct{})
	var senders sync.WaitGroup
	for _, c := range agentsConns {
		senders.Add(1)
		go func(c *websocket.Conn) {
			defer senders.Done()
			tick := time.NewTicker(*interval)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					c.SetWriteDeadline(time.Now().Add(5 * time.Second))
					if err := c.WriteJSON(drite.MetricMessage{CPUUsage: 50, MemoryUsage: 40, NetworkSpeed: 10000}); err != nil {
						if !stopping.Load() {
							failures.Add(1)
						}
						return
					}
					sent.Add(1)
				}
			}
		}(c)
	}
	timer := time.NewTimer(*duration)
	<-timer.C
	close(stop)
	senders.Wait()
	elapsed := time.Since(start)
	// Give the hub time to drain. Incomplete delivery remains visible in the report.
	expected := sent.Load() * int64(*dashboards)
	deadline := time.Now().Add(3 * time.Second)
	for received.Load() < expected && failures.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	stopping.Store(true)
	for _, c := range conns {
		c.Close()
	}
	wg.Wait()
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(float64(len(latencies)-1)*p)]
	}
	report := map[string]any{"agents": *agents, "dashboards": *dashboards, "elapsed_seconds": elapsed.Seconds(), "sent_metrics": sent.Load(), "expected_deliveries": expected, "received_deliveries": received.Load(), "connection_errors": failures.Load(), "sent_per_second": float64(sent.Load()) / elapsed.Seconds(), "server_to_subscriber_p50_ms": percentile(.50), "server_to_subscriber_p95_ms": percentile(.95), "server_to_subscriber_p99_ms": percentile(.99), "latency_sample_count": len(latencies), "latency_sample_limit": 100000}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if failures.Load() > 0 || received.Load() != expected {
		return fmt.Errorf("workload did not achieve complete delivery; inspect the report")
	}
	return nil
}
