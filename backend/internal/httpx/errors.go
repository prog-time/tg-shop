package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorCode is a stable machine-readable error code, matching the
// `Error.error.code` enum in docs/api/openapi.yaml (components/schemas/Error).
type ErrorCode string

// Error codes from the contract's shared error envelope. Keep this list in
// sync with docs/api/openapi.yaml components/schemas/Error.
const (
	ErrCodeValidation   ErrorCode = "VALIDATION_ERROR"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeConflict     ErrorCode = "CONFLICT"
	ErrCodeRateLimited  ErrorCode = "RATE_LIMITED"
	ErrCodeInternal     ErrorCode = "INTERNAL_ERROR"
)

// ErrorDetail is one item of Error.error.details: a single field-level
// validation failure.
type ErrorDetail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// errorBody is the wire shape of the contract's unified error envelope:
// { "error": { "code", "message", "details": [...] } }.
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    ErrorCode     `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

// WriteError writes the unified error envelope with the given HTTP status,
// code and message. It is the one place in the codebase that produces an
// error response body, so every handler stays consistent with the contract.
func WriteError(w http.ResponseWriter, status int, code ErrorCode, message string, details ...ErrorDetail) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorPayload{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

// NotImplemented writes a 501 in the unified error envelope. It is the
// placeholder response for every contract operation that has no domain
// handler wired up yet (see cmd/api's catch-all mount under /api/v1).
//
// It reports ErrCodeInternal because the contract's Error.error.code enum
// (docs/api/openapi.yaml components/schemas/Error) has no "not implemented"
// member — INTERNAL_ERROR is the closest legal value until/unless the
// contract grows one.
func NotImplemented(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, ErrCodeInternal,
		"this endpoint is defined by the API contract but not implemented yet")
}

// InternalError writes a 500 in the unified error envelope. It never leaks
// the underlying error to the client — callers are expected to have already
// logged it (e.g. the Recoverer middleware).
func InternalError(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternal, "internal server error")
}
