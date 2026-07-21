package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// RefreshTokenTTL is the staff refresh token lifetime, per the approved
// decision (30 days).
const RefreshTokenTTL = 30 * 24 * time.Hour

// refreshTokenBytes is the raw entropy of a generated refresh token before
// encoding (32 bytes = 256 bits), comfortably beyond any brute-force
// concern — the token is opaque and never derived from anything guessable.
const refreshTokenBytes = 32

// GenerateRefreshToken returns a new high-entropy opaque refresh token
// (raw, handed to the client once and never stored) and its hash
// (persisted in admin_refresh_tokens.token_hash — see HashRefreshToken).
func GenerateRefreshToken() (raw string, hash string, err error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashRefreshToken(raw), nil
}

// HashRefreshToken deterministically hashes a raw refresh token for storage
// and lookup. SHA-256 (not argon2id) is deliberate here: unlike a
// user-chosen password, the input already has 256 bits of entropy, so a
// slow, memory-hard KDF buys nothing against brute force and would only
// slow down the point lookup on admin_refresh_tokens.token_hash (which has
// a unique index — the query is an equality match, so the hash must be
// deterministic; a salted/slow hash like bcrypt or argon2id cannot support
// that index at all).
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
