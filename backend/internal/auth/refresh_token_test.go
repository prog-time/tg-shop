package auth

import "testing"

func TestGenerateRefreshToken_HashMatchesRawAndTokensAreUnique(t *testing.T) {
	raw1, hash1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	raw2, hash2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	if raw1 == raw2 {
		t.Fatal("expected two generated refresh tokens to differ")
	}
	if hash1 != HashRefreshToken(raw1) {
		t.Fatal("hash1 does not match HashRefreshToken(raw1)")
	}
	if hash2 != HashRefreshToken(raw2) {
		t.Fatal("hash2 does not match HashRefreshToken(raw2)")
	}
	if hash1 == hash2 {
		t.Fatal("expected two distinct raw tokens to hash differently")
	}
}

func TestHashRefreshToken_Deterministic(t *testing.T) {
	if HashRefreshToken("same-input") != HashRefreshToken("same-input") {
		t.Fatal("expected HashRefreshToken to be deterministic for the same input")
	}
}
