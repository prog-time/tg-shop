// Package httpx provides the shared HTTP plumbing for all services: a base
// router with liveness/readiness/metrics endpoints and a graceful server run
// loop. Domain routes are mounted onto the base router by each service.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Checker reports whether a dependency is reachable. It powers /readyz.
type Checker func(ctx context.Context) error

// BaseRouter returns a router exposing /healthz (liveness), /readyz
// (readiness — runs every checker) and /metrics (Prometheus). ready may be nil.
func BaseRouter(ready map[string]Checker) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		res := make(map[string]string, len(ready))
		code := http.StatusOK
		for name, check := range ready {
			if err := check(req.Context()); err != nil {
				res[name] = err.Error()
				code = http.StatusServiceUnavailable
				continue
			}
			res[name] = "ok"
		}
		writeJSON(w, code, res)
	})

	r.Handle("/metrics", promhttp.Handler())
	return r
}

// Run starts srv and blocks until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, log *slog.Logger, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
