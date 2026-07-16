// Command bot is the Telegram bot entrypoint. It receives updates by webhook
// (the secret in the path keeps strangers out) and forwards business events to
// api. In this skeleton it only boots, serves health/metrics and acks the
// webhook; update handling arrives in a later phase.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prog-time/tg-shop/backend/internal/config"
	"github.com/prog-time/tg-shop/backend/internal/httpx"
	"github.com/prog-time/tg-shop/backend/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r := httpx.BaseRouter(nil)
	if cfg.BotWebhookSecret != "" {
		r.Post("/tg/"+cfg.BotWebhookSecret, func(w http.ResponseWriter, _ *http.Request) {
			// Update handling is implemented in a later phase; ack for now.
			w.WriteHeader(http.StatusOK)
		})
	}

	return httpx.Run(ctx, log, config.EnvOr("BOT_ADDR", ":8081"), r)
}
