// Package config loads all runtime configuration from ENV. Per the project
// rule, every service is configured only through environment variables; the
// connection URLs are assembled in docker-compose from host/port/credentials
// so a managed cloud service can be swapped in without code changes.
package config

import (
	"fmt"
	"os"
)

// Config is the full set of settings shared by api, bot and worker. Each
// binary uses the subset it needs and validates required fields explicitly.
type Config struct {
	LogLevel string

	DatabaseURL string
	RedisURL    string
	RabbitURL   string

	S3Endpoint     string
	S3PublicURL    string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool

	BotToken         string
	BotWebhookSecret string
	JWTSecret        string
	PublicURL        string
}

// Load reads configuration from the environment. It never fails on missing
// optional values; per-service validation (e.g. RequireDB) is explicit.
func Load() (*Config, error) {
	return &Config{
		LogLevel:         EnvOr("LOG_LEVEL", "info"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		RabbitURL:        os.Getenv("RABBITMQ_URL"),
		S3Endpoint:       os.Getenv("S3_ENDPOINT"),
		S3PublicURL:      os.Getenv("S3_PUBLIC_URL"),
		S3Region:         EnvOr("S3_REGION", "us-east-1"),
		S3Bucket:         os.Getenv("S3_BUCKET"),
		S3AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:      os.Getenv("S3_SECRET_KEY"),
		S3UsePathStyle:   EnvOr("S3_USE_PATH_STYLE", "true") == "true",
		BotToken:         os.Getenv("BOT_TOKEN"),
		BotWebhookSecret: os.Getenv("BOT_WEBHOOK_SECRET"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		PublicURL:        os.Getenv("PUBLIC_URL"),
	}, nil
}

// RequireDB validates the fields a service needs to reach PostgreSQL.
func (c *Config) RequireDB() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

// RequireAuth validates the fields the Auth Module needs: BOT_TOKEN backs
// the HMAC secret key for storefront initData verification
// (internal/auth/initdata.go), JWT_SECRET signs/verifies the staff access
// token (internal/auth/jwt.go). Neither has a safe default — starting api
// with either empty would silently accept a forged initData signature or
// sign every admin session with a well-known key, so api refuses to start
// rather than degrade quietly.
func (c *Config) RequireAuth() error {
	if c.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	return nil
}

// EnvOr returns the value of key k, or def when unset or empty.
func EnvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
