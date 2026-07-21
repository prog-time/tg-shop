package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InitDataMaxAge is the freshness window for Telegram Web App `initData`:
// anything with an `auth_date` older than this is rejected as stale, even if
// the signature is valid. 24h is a judgment call — Telegram does not mandate
// a value. It is generous enough that a customer who opens the Web App and
// leaves the tab open overnight isn't logged out, while still bounding how
// long a captured initData string (e.g. leaked via logs, a proxy, browser
// history) remains replayable, since initData carries no nonce of its own.
const InitDataMaxAge = 24 * time.Hour

// webAppDataKey is the fixed HMAC key Telegram specifies for deriving the
// per-bot secret key. It is not a secret itself — every bot uses the same
// literal string; the actual secret is BOT_TOKEN.
const webAppDataKey = "WebAppData"

// ErrInitDataInvalid is returned by VerifyInitData for any failure: missing
// hash, malformed payload, signature mismatch, or stale auth_date. The
// specific reason is never surfaced to the client (per Security rules,
// verification detail must not leak) but is safe to log server-side.
var ErrInitDataInvalid = errors.New("initdata: invalid or stale")

// tgUserJSON mirrors the subset of Telegram's WebAppUser JSON the shop cares
// about. Only fields present in `docs/api/openapi.yaml`'s Customer schema are
// extracted; everything else in the `user` field is ignored.
type tgUserJSON struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// VerifyInitData verifies raw (the query-string-encoded initData payload
// handed to the storefront by the Telegram client SDK, sent as
// `Authorization: tma <initData>`) against botToken, per Telegram's
// documented algorithm:
//
//  1. secret_key = HMAC_SHA256(key="WebAppData", data=botToken)
//  2. data_check_string = every "key=value" pair except "hash", sorted by
//     key, joined with "\n"
//  3. expected_hash = HMAC_SHA256(key=secret_key, data=data_check_string)
//  4. expected_hash must equal the payload's "hash" field (constant-time)
//  5. "auth_date" must be within maxAge of now
//
// Verification happens exclusively here, server-side, per
// docs/architecture.md's Security rules — there is no client-side
// counterpart and none should be added. On any failure it returns
// ErrInitDataInvalid; the caller (RequireInitData) maps that to a uniform
// 401, never distinguishing "missing" from "tampered" from "stale" to the
// client.
func VerifyInitData(botToken, raw string, maxAge time.Duration, now time.Time) (TelegramUser, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return TelegramUser{}, ErrInitDataInvalid
	}

	providedHash := values.Get("hash")
	if providedHash == "" {
		return TelegramUser{}, ErrInitDataInvalid
	}
	values.Del("hash")

	authDateRaw := values.Get("auth_date")
	if authDateRaw == "" {
		return TelegramUser{}, ErrInitDataInvalid
	}
	authDateUnix, err := strconv.ParseInt(authDateRaw, 10, 64)
	if err != nil {
		return TelegramUser{}, ErrInitDataInvalid
	}
	authDate := time.Unix(authDateUnix, 0)
	if now.Sub(authDate) > maxAge || authDate.After(now.Add(5*time.Minute)) {
		// The small forward-skew allowance covers clock drift between the
		// Telegram server that stamped auth_date and this host; anything
		// beyond that is treated the same as "too old".
		return TelegramUser{}, ErrInitDataInvalid
	}

	dataCheckString := buildDataCheckString(values)

	secretKey := hmac.New(sha256.New, []byte(webAppDataKey))
	secretKey.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secretKey.Sum(nil))
	mac.Write([]byte(dataCheckString))
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedHash), []byte(strings.ToLower(providedHash))) {
		return TelegramUser{}, ErrInitDataInvalid
	}

	userRaw := values.Get("user")
	if userRaw == "" {
		return TelegramUser{}, ErrInitDataInvalid
	}
	var u tgUserJSON
	if err := json.Unmarshal([]byte(userRaw), &u); err != nil || u.ID == 0 {
		return TelegramUser{}, ErrInitDataInvalid
	}

	return TelegramUser{
		ID:        u.ID,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
	}, nil
}

func buildDataCheckString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+values.Get(k))
	}
	return strings.Join(pairs, "\n")
}
