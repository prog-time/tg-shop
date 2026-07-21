package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// buildSignedInitData independently replicates Telegram's documented
// initData signing algorithm (sort keys, join "k=v" pairs with "\n", HMAC
// SHA-256 twice) without calling any unexported helper from initdata.go —
// it exercises VerifyInitData/RequireInitData from the outside, the same
// way the Telegram client SDK produces the string.
func buildSignedInitData(t *testing.T, botToken string, fields map[string]string) string {
	t.Helper()

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+fields[k])
	}
	dataCheckString := strings.Join(pairs, "\n")

	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secretKey.Sum(nil))
	mac.Write([]byte(dataCheckString))
	hash := hex.EncodeToString(mac.Sum(nil))

	values := url.Values{}
	for k, v := range fields {
		values.Set(k, v)
	}
	values.Set("hash", hash)
	return values.Encode()
}

func unixString(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

// flipLastHexChar corrupts a valid, signed initData string's hash so its
// signature no longer verifies, without touching any other field —
// simulating a tampered/forged payload.
func flipLastHexChar(t *testing.T, raw string) string {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	h := values.Get("hash")
	if h == "" {
		t.Fatal("expected a hash field to corrupt")
	}
	last := h[len(h)-1]
	replacement := byte('0')
	if last == '0' {
		replacement = '1'
	}
	values.Set("hash", h[:len(h)-1]+string(replacement))
	return values.Encode()
}

func TestVerifyInitData_Valid(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42,"username":"neo","first_name":"Thomas","last_name":"Anderson"}`,
		"query_id":  "AAHexample",
	})

	u, err := VerifyInitData(botToken, raw, InitDataMaxAge, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyInitData: %v", err)
	}
	if u.ID != 42 || u.Username != "neo" || u.FirstName != "Thomas" || u.LastName != "Anderson" {
		t.Fatalf("unexpected TelegramUser: %+v", u)
	}
}

func TestVerifyInitData_InvalidHash(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42}`,
	})
	tampered := flipLastHexChar(t, raw)

	if _, err := VerifyInitData(botToken, tampered, InitDataMaxAge, now); err == nil {
		t.Fatal("expected an error for a tampered hash")
	}
}

func TestVerifyInitData_WrongBotToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, "correct-bot-token", map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42}`,
	})

	if _, err := VerifyInitData("different-bot-token", raw, InitDataMaxAge, now); err == nil {
		t.Fatal("expected an error when verifying against the wrong bot token")
	}
}

func TestVerifyInitData_StaleAuthDate(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	stale := now.Add(-InitDataMaxAge - time.Hour)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(stale),
		"user":      `{"id":42}`,
	})

	if _, err := VerifyInitData(botToken, raw, InitDataMaxAge, now); err == nil {
		t.Fatal("expected an error for a stale auth_date")
	}
}

func TestVerifyInitData_TamperedPayload(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42}`,
	})

	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	// Change the payload after signing, without recomputing the hash —
	// the signature must no longer match.
	values.Set("user", `{"id":99}`)
	tampered := values.Encode()

	if _, err := VerifyInitData(botToken, tampered, InitDataMaxAge, now); err == nil {
		t.Fatal("expected an error for a tampered payload")
	}
}

func TestVerifyInitData_MissingHash(t *testing.T) {
	values := url.Values{}
	values.Set("auth_date", unixString(time.Unix(1_700_000_000, 0)))
	values.Set("user", `{"id":42}`)

	if _, err := VerifyInitData("bot-token", values.Encode(), InitDataMaxAge, time.Unix(1_700_000_000, 0)); err == nil {
		t.Fatal("expected an error when hash is missing")
	}
}

func TestVerifyInitData_MissingUser(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
	})

	if _, err := VerifyInitData(botToken, raw, InitDataMaxAge, now); err == nil {
		t.Fatal("expected an error when the user field is missing")
	}
}
