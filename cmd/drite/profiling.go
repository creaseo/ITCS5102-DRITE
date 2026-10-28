package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
)

// Profiling is opt-in and restricted to loopback; it is never on the public API.
func startProfiler(address string) (func(), error) {
	if address == "" {
		return func() {}, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid profiling address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("profiling requires a loopback IP address")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("local profiling on %s", address)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("profiler: %v", err)
		}
	}()
	return func() { _ = server.Close() }, nil
}
