package auth

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashPassword_VerifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected the correct password to verify")
	}
}

func TestVerifyPassword_WrongPasswordRejected(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("expected the wrong password to be rejected")
	}
}

func TestHashPassword_UniqueSaltPerCall(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Fatal("expected two hashes of the same password to differ (random salt)")
	}
}

func TestVerifyPassword_MalformedHashHandled(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash-at-all",
		"$argon2id$v=19$m=65536,t=3,p=4$onlyfourparts",
		"$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
	}
	for _, encoded := range cases {
		if _, err := VerifyPassword("anything", encoded); err == nil {
			t.Fatalf("expected an error for malformed hash %q, got none", encoded)
		}
	}
}

func TestDummyPasswordHash_IsValidAndNeverMatchesRealInput(t *testing.T) {
	ok, err := VerifyPassword("this-is-not-a-real-password-used-only-for-timing", dummyPasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword against dummy hash: %v", err)
	}
	if !ok {
		t.Fatal("dummyPasswordHash should verify against its own known plaintext")
	}

	ok, err = VerifyPassword("some random guess", dummyPasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("dummyPasswordHash must not verify against an arbitrary password")
	}
}

func TestVerifyPassword_RejectsForeignArgonVersion(t *testing.T) {
	encoded, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Same hash, relabelled with a version this build cannot reproduce. The
	// derived key would differ, so silently verifying it would reject a
	// correct password as if the user had mistyped it.
	tampered := strings.Replace(encoded, fmt.Sprintf("$v=%d$", argon2.Version), "$v=13579$", 1)
	if tampered == encoded {
		t.Fatalf("failed to rewrite version in %q", encoded)
	}

	ok, err := VerifyPassword("correct-password", tampered)
	if err == nil {
		t.Fatal("expected an error for an unsupported argon2 version, got nil")
	}
	if ok {
		t.Fatal("a hash with an unsupported version must never report a match")
	}
}
