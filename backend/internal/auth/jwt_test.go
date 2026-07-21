package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseAccessToken(t *testing.T) {
	secret := []byte("test-secret")
	// ParseAccessToken validates `exp` against the real wall clock (the
	// jwt/v5 library has no injectable clock here), so the token must be
	// signed relative to real "now", not a fixed timestamp.
	now := time.Now()

	token, err := IssueAccessToken(secret, 7, "admin", now)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	claims, err := ParseAccessToken(secret, token)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.AdminID != 7 || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	// jwt's NumericDate truncates to whole seconds, so compare at that
	// resolution rather than exact time.Time equality.
	if claims.ExpiresAt.Unix() != now.Add(AccessTokenTTL).Unix() {
		t.Fatalf("expires_at = %v, want %v", claims.ExpiresAt.Time, now.Add(AccessTokenTTL))
	}
}

func TestParseAccessToken_ExpiredRejected(t *testing.T) {
	secret := []byte("test-secret")
	longAgo := time.Now().Add(-2 * AccessTokenTTL)

	token, err := IssueAccessToken(secret, 7, "admin", longAgo)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	if _, err := ParseAccessToken(secret, token); err == nil {
		t.Fatal("expected an error for an expired token")
	}
}

func TestParseAccessToken_BadSignatureRejected(t *testing.T) {
	token, err := IssueAccessToken([]byte("secret-a"), 7, "admin", time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	if _, err := ParseAccessToken([]byte("secret-b"), token); err == nil {
		t.Fatal("expected an error for a token signed with a different secret")
	}
}

func TestParseAccessToken_MalformedRejected(t *testing.T) {
	if _, err := ParseAccessToken([]byte("secret"), "not.a.jwt"); err == nil {
		t.Fatal("expected an error for a malformed token string")
	}
}
