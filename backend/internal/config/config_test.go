package config

import (
	"strings"
	"testing"
)

// TestRequireAuth covers the gate that keeps api from starting with an
// unusable auth configuration. The placeholder and length checks exist
// because "non-empty" was not a real bar: `.env.example` ships
// JWT_SECRET=change-me, so a deployment that copied the example verbatim
// would have signed every staff session with a value published in this
// repository.
func TestRequireAuth(t *testing.T) {
	const goodSecret = "0Gm4Zr9xQeT2sVbN7pLkJhF3dWcYaXuS" // exactly 32 chars

	tests := []struct {
		name      string
		botToken  string
		jwtSecret string
		wantErr   string // empty means RequireAuth must succeed
	}{
		{
			name:      "accepts a long random secret",
			botToken:  "123456:AAbbCC",
			jwtSecret: goodSecret,
		},
		{
			name:      "rejects a missing bot token",
			jwtSecret: goodSecret,
			wantErr:   "BOT_TOKEN is required",
		},
		{
			name:     "rejects a missing jwt secret",
			botToken: "123456:AAbbCC",
			wantErr:  "JWT_SECRET is required",
		},
		{
			name:      "rejects the example placeholder",
			botToken:  "123456:AAbbCC",
			jwtSecret: "change-me",
			wantErr:   "placeholder",
		},
		{
			name:      "rejects the placeholder regardless of case or padding",
			botToken:  "123456:AAbbCC",
			jwtSecret: "  Change-Me  ",
			wantErr:   "placeholder",
		},
		{
			name:      "rejects a secret too short to resist offline brute force",
			botToken:  "123456:AAbbCC",
			jwtSecret: "s3cr3t-but-tiny",
			wantErr:   "at least 32 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{BotToken: tt.botToken, JWTSecret: tt.jwtSecret}
			err := c.RequireAuth()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("RequireAuth: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("RequireAuth: expected an error mentioning %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("RequireAuth error = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
