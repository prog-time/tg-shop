package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	var gotID string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID = GetRequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotID == "" {
		t.Fatal("expected a generated request id in context")
	}
	if got := rec.Header().Get(RequestIDHeader); got != gotID {
		t.Fatalf("response header = %q, want %q", got, gotID)
	}
}

func TestRequestID_PreservesInbound(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "caller-supplied-id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "caller-supplied-id" {
		t.Fatalf("request id = %q, want %q", got, "caller-supplied-id")
	}
}

func TestLogging_IncludesRequestIDAndStatus(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	h := RequestID(Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("invalid log line: %v, raw: %s", err, buf.String())
	}
	if line["status"] != float64(http.StatusTeapot) {
		t.Fatalf("status = %v, want %d", line["status"], http.StatusTeapot)
	}
	if line["path"] != "/foo" {
		t.Fatalf("path = %v, want /foo", line["path"])
	}
	if id, _ := line["request_id"].(string); id == "" {
		t.Fatalf("expected non-empty request_id in log line, got %v", line["request_id"])
	}
}

func TestRecoverer_RecoversPanicAndWritesEnvelope(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	h := RequestID(Recoverer(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req) // must not panic out of the test

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected error envelope in body, got %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Fatalf("expected the panic to be logged, got %s", buf.String())
	}
}

func TestRecoverer_PassesThroughWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no log output on the happy path, got %s", buf.String())
	}
}
