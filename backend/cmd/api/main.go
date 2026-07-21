// Command api is the Backend API: all business logic and the single owner of
// the database. It applies migrations on startup, then serves HTTP.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prog-time/tg-shop/backend/internal/auth"
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
	if err := cfg.RequireAuth(); err != nil {
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

	r := newRouter(log, map[string]httpx.Checker{
		"postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
		"redis":    func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
	}, cfg, pool)

	return httpx.Run(ctx, log, config.EnvOr("API_ADDR", ":8080"), r)
}

// newRouter builds the full HTTP router: liveness/readiness/metrics (as
// before) plus the contract surface mounted under /api/v1. cfg supplies the
// Auth Module's secrets (BOT_TOKEN, JWT_SECRET); pool backs internal/auth's
// Repo. pool may be nil in tests that only exercise routes without a domain
// handler behind them (e.g. /healthz, the 501 catch-all) — auth.NewRepo
// wraps it lazily and nothing in those tests calls a Repo method.
func newRouter(log *slog.Logger, ready map[string]httpx.Checker, cfg *config.Config, pool *pgxpool.Pool) *chi.Mux {
	r := httpx.BaseRouter(ready)

	api := chi.NewRouter()
	api.Use(httpx.RequestID)
	api.Use(httpx.Logging(log))
	api.Use(httpx.Recoverer(log))

	authRepo := auth.NewRepo(pool)
	authHandlers := &auth.Handlers{
		Service: &auth.Service{Repo: authRepo, JWTSecret: []byte(cfg.JWTSecret)},
		Log:     log,
	}
	authHandlers.Mount(api, cfg.BotToken, []byte(cfg.JWTSecret))

	// Every other domain handler is still unimplemented (see
	// docs/api/openapi.yaml, 49 paths / 79 ops). Domain routers implement
	// openapi.StrictServerInterface (internal/openapi/openapi.gen.go,
	// generated via `go generate ./...` per ADR-005) and mount their routes
	// here, incrementally replacing this catch-all as each domain slice
	// lands (issue #6+). Storefront routes apply auth.RequireInitData,
	// admin routes apply auth.RequireAdminJWT (internal/auth), matching the
	// `initData`/`adminJWT` security schemes in docs/api/openapi.yaml — the
	// auth module itself (issue #5) is the first slice wired for real.
	api.HandleFunc("/*", httpx.NotImplemented)
	api.NotFound(httpx.NotImplemented)
	api.MethodNotAllowed(httpx.NotImplemented)

	r.Mount("/api/v1", api)
	return r
}
