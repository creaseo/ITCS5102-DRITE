// A synthetic telemetry agent for demonstrations; it does not measure the host.
package main

import (
	"context"
	"flag"
	drite "github.com/creaseo/DRITE"
	"github.com/gorilla/websocket"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"
)

func main() {
	endpoint := flag.String("url", "ws://localhost:8080/ws/agent", "agent WebSocket URL")
	id := flag.String("id", "demo-node", "agent identifier")
	interval := flag.Duration("interval", time.Second, "sampling interval")
	cpu := flag.Float64("cpu", -1, "fixed CPU percentage; -1 generates random values")
	flag.Parse()
	if *interval <= 0 || math.IsNaN(*cpu) || math.IsInf(*cpu, 0) || (*cpu < 0 && *cpu != -1) || *cpu > 100 {
		log.Fatal("interval must be positive; cpu must be -1 or in [0,100]")
	}
	u, err := url.Parse(*endpoint)
	if err != nil {
		log.Fatal(err)
	}
	q := u.Query()
	q.Set("agent_id", *id)
	u.RawQuery = q.Encode()
	token := os.Getenv("DRITE_AUTH_TOKEN")
	if token == "" {
		token = "drite-demo-token"
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	lost := make(chan error, 1)
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				lost <- err
				return
			}
		}
	}() // Reads are required to answer Ping frames.
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	log.Printf("streaming synthetic metrics as %s", *id)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-lost:
			log.Printf("connection ended: %v", err)
			return
		case <-ticker.C:
			usage := *cpu
			if usage < 0 {
				usage = 20 + rand.Float64()*80
			}
			m := drite.MetricMessage{AgentID: *id, CPUUsage: usage, MemoryUsage: 30 + rand.Float64()*50, NetworkSpeed: rand.Float64() * 1000000}
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.WriteJSON(m); err != nil {
				log.Printf("write: %v", err)
				return
			}
		}
	}
}
