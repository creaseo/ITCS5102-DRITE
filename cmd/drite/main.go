package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	drite "github.com/creaseo/DRITE/internal/drite"
)

func main() {
	profileAddress := flag.String("pprof", "", "optional loopback profiling address, e.g. 127.0.0.1:6060")
	flag.Parse()
	stopProfiler, err := startProfiler(*profileAddress)
	if err != nil {
		log.Fatal(err)
	}
	defer stopProfiler()
	cfg, address, err := drite.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	app := drite.NewServer(cfg)
	server := &http.Server{Addr: address, Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 1)
	go func() { log.Printf("DRITE listening on %s", address); failures <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err = <-failures:
		if err != http.ErrServerClosed {
			log.Printf("HTTP server: %v", err)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := server.Shutdown(shutdown); e != nil {
		log.Printf("HTTP shutdown: %v", e)
		_ = server.Close()
	}
	app.Close() // HTTP Shutdown does not close upgraded WebSocket connections.
	if err != nil && err != http.ErrServerClosed {
		os.Exit(1)
	}
}
