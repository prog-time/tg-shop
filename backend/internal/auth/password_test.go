package auth

import "testing"

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
