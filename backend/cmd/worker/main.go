// Command worker runs background jobs: the outbox relay and the RabbitMQ
// consumers (notifications, payments, facet rebuilds). It has no inbound routes
// exposed by the proxy — only /healthz and /metrics for observability.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prog-time/tg-shop/backend/internal/config"
	"github.com/prog-time/tg-shop/backend/internal/httpx"
	"github.com/prog-time/tg-shop/backend/internal/logging"
	"github.com/prog-time/tg-shop/backend/internal/postgres"
	"github.com/prog-time/tg-shop/backend/internal/rabbit"
)

var errRabbitClosed = errors.New("rabbitmq connection closed")

func main() {
	if err := run(); err != nil {
		slog.Error("worker exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel)

	if err := cfg.RequireDB(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	conn, err := rabbit.Dial(cfg.RabbitURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Placeholder loop. The outbox relay and queue consumers are implemented in
	// a later phase; for now the worker just proves it boots and stays healthy.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				log.Debug("worker tick — outbox relay / consumers not yet implemented")
			}
		}
	}()

	r := httpx.BaseRouter(map[string]httpx.Checker{
		"postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
		"rabbitmq": func(context.Context) error {
			if conn.IsClosed() {
				return errRabbitClosed
			}
			return nil
		},
	})

	return httpx.Run(ctx, log, config.EnvOr("WORKER_ADDR", ":8082"), r)
}
