// Package redisx owns the Redis connection. Redis is cache and cart storage
// only — never a queue (see ADR-004).
package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// New parses the Redis URL, opens a client and verifies connectivity.
func New(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return c, nil
}
