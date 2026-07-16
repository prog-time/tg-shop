// Command api is the Backend API: all business logic and the single owner of
// the database. It applies migrations on startup, then serves HTTP.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prog-time/tg-shop/backend/internal/config"
	"github.com/prog-time/tg-shop/backend/internal/httpx"
	"github.com/prog-time/tg-shop/backend/internal/logging"
	"github.com/prog-time/tg-shop/backend/internal/postgres"
	redisx "github.com/prog-time/tg-shop/backend/internal/redis"
	"github.com/prog-time/tg-shop/backend/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", "err", err)
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

	// api is the sole owner of the schema — apply migrations before serving.
	if err := postgres.Migrate(cfg.DatabaseURL, migrations.FS); err != nil {
		return err
	}
	log.Info("migrations applied")

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb, err := redisx.New(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	r := httpx.BaseRouter(map[string]httpx.Checker{
		"postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
		"redis":    func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
	})
	// Domain routes (contract-first via oapi-codegen) are mounted here in later phases.

	return httpx.Run(ctx, log, config.EnvOr("API_ADDR", ":8080"), r)
}
