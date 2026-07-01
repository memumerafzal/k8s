// hello-k8s is a tiny, dependency-free web service built to demonstrate
// production-shaped Kubernetes practices: real health/readiness endpoints,
// graceful shutdown, downward-API awareness, and a minimal Prometheus metric.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// version is injected at build time via -ldflags "-X main.version=...".
// It can be overridden at runtime by the VERSION env var (set from the image tag).
var version = "dev"

// ready flips to true once warm-up completes and back to false on SIGTERM,
// so the readiness probe actually reflects whether we should receive traffic.
var ready atomic.Bool

// requests is a naive request counter exposed on /metrics in Prometheus format.
var requests atomic.Int64

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if v := os.Getenv("VERSION"); v != "" {
		version = v
	}

	addr := ":" + env("PORT", "8080")
	readyDelay, _ := time.ParseDuration(env("READY_DELAY", "3s"))

	info := map[string]string{
		"app":       "hello-k8s",
		"version":   version,
		"pod":       env("POD_NAME", "local"),
		"namespace": env("POD_NAMESPACE", "local"),
		"node":      env("NODE_NAME", "local"),
	}

	mux := http.NewServeMux()

	// Liveness: is the process alive? Cheap and always-200 while running.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	// Readiness: should we receive traffic? False during warm-up and drain.
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, info)
	})

	// Minimal Prometheus exposition — no client library, stdlib only.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "# HELP hello_requests_total Total requests served on /\n")
		fmt.Fprintf(w, "# TYPE hello_requests_total counter\n")
		fmt.Fprintf(w, "hello_requests_total %d\n", requests.Load())
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		writeJSON(w, map[string]any{
			"message": "Hello from Kubernetes 👋",
			"served":  info,
			"count":   requests.Load(),
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Simulate warm-up so the readiness probe is meaningful in a demo.
	go func() {
		time.Sleep(readyDelay)
		ready.Store(true)
		log.Printf("ready after %s warm-up", readyDelay)
	}()

	// Graceful shutdown: on SIGTERM, stop advertising readiness, give the
	// service endpoints time to drain, then shut the server down cleanly.
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		<-stop
		log.Println("shutdown signal received; draining")
		ready.Store(false)
		time.Sleep(3 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown error: %v", err)
		}
	}()

	log.Printf("hello-k8s %s listening on %s (pod=%s)", version, addr, info["pod"])
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("stopped")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
