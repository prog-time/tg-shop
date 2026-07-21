package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prog-time/tg-shop/backend/internal/config"
	"github.com/prog-time/tg-shop/backend/internal/httpx"
	"github.com/prog-time/tg-shop/backend/internal/logging"
)

// testConfig is a minimal Auth-Module-capable config for router tests that
// don't have a live Postgres connection (pool is passed as nil — see
// newRouter's doc comment). The secrets are dummy values; nothing in these
// tests exercises real cryptography against them.
func testConfig() *config.Config {
	return &config.Config{
		BotToken:  "test-bot-token",
		JWTSecret: "test-jwt-secret",
	}
}

func TestHealthzStillReturns200(t *testing.T) {
	r := newRouter(logging.New("error"), nil, testConfig(), nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUnimplementedContractRouteReturns501Envelope(t *testing.T) {
	r := newRouter(logging.New("error"), nil, testConfig(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
	req.Header.Set(httpx.RequestIDHeader, "test-request-id")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	if got := rec.Header().Get(httpx.RequestIDHeader); got != "test-request-id" {
		t.Fatalf("request id header = %q, want %q", got, "test-request-id")
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v, raw: %s", err, rec.Body.String())
	}
	if body.Error.Code == "" || body.Error.Message == "" {
		t.Fatalf("expected a populated error envelope, got %+v", body)
	}
}

func TestUnimplementedContractRoute_AnyMethodAndDeepPath(t *testing.T) {
	r := newRouter(logging.New("error"), nil, testConfig(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products/123/some/unknown/nesting", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}
