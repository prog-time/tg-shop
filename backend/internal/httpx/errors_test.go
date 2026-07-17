package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError_WithDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, ErrCodeValidation, "bad input",
		ErrorDetail{Field: "email", Issue: "required"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Error.Code != ErrCodeValidation {
		t.Fatalf("code = %q, want %q", body.Error.Code, ErrCodeValidation)
	}
	if body.Error.Message != "bad input" {
		t.Fatalf("message = %q, want %q", body.Error.Message, "bad input")
	}
	if len(body.Error.Details) != 1 || body.Error.Details[0].Field != "email" {
		t.Fatalf("unexpected details: %+v", body.Error.Details)
	}
}

func TestWriteError_OmitsEmptyDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusInternalServerError, ErrCodeInternal, "boom")

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	errObj, _ := raw["error"].(map[string]any)
	if _, present := errObj["details"]; present {
		t.Fatalf("expected no details field when none given, got %v", errObj)
	}
}

func TestNotImplemented_ReturnsEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	NotImplemented(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Error.Code != ErrCodeInternal {
		t.Fatalf("code = %q, want %q", body.Error.Code, ErrCodeInternal)
	}
	if body.Error.Message == "" {
		t.Fatal("expected a non-empty message")
	}
}

func TestInternalError_ReturnsEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	InternalError(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Error.Code != ErrCodeInternal {
		t.Fatalf("code = %q, want %q", body.Error.Code, ErrCodeInternal)
	}
}
