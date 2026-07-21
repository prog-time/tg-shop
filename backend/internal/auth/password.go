package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. Judgment call: this is OWASP's documented
// "m=64 MiB (65536 KiB), t=3, p=4" recommendation for interactive login
// (OWASP Password Storage Cheat Sheet, Argon2id section) — a reasonable
// default with no project-specific latency budget to tune against yet.
// keyLength/saltLength (32/16 bytes) match the reference argon2id
// implementations this cheat sheet is based on.
const (
	argonMemoryKiB   = 64 * 1024
	argonIterations  = 3
	argonParallelism = 4
	argonSaltLength  = 16
	argonKeyLength   = 32
)

// dummyPasswordHash is a valid argon2id hash of an unknown, unused password.
// Login verifies against this when the looked-up account doesn't exist, so a
// failed lookup costs the same argon2id computation as a failed verify —
// keeping the two paths timing-indistinguishable (see Service.Login).
var dummyPasswordHash = mustHashPassword("this-is-not-a-real-password-used-only-for-timing")

// HashPassword returns an encoded argon2id hash of password:
// "$argon2id$v=<version>$m=<mem>,t=<iter>,p=<par>$<salt-b64>$<hash-b64>",
// matching the widely-used PHC string format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonKeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func mustHashPassword(password string) string {
	h, err := HashPassword(password)
	if err != nil {
		panic(err)
	}
	return h
}

// VerifyPassword reports whether password matches encoded (as produced by
// HashPassword), using a constant-time comparison of the derived key so
// timing cannot leak how many bytes matched.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errors.New("auth: unrecognized password hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("parse version: %w", err)
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1, nil
}
