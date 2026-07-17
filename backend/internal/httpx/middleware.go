package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestIDHeader is both the inbound header consulted for a caller-supplied
// correlation id and the header the response echoes it back on.
const RequestIDHeader = "X-Request-Id"

type contextKey int

const requestIDKey contextKey = iota

// RequestID accepts an inbound X-Request-Id header, or generates one, and
// makes it available via GetRequestID and on the response header. It must be
// the outermost middleware in the chain so every later middleware (and
// handler) can log with the id.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request id stored in ctx by RequestID, or "" if
// none is present (e.g. outside of an HTTP request).
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively unrecoverable, but a missing
		// request id must never break request handling.
		return "unavailable"
	}
	return hex.EncodeToString(b[:])
}

// Logging logs one structured line per request via slog: method, path,
// status, duration and request id. Mount it after RequestID.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", GetRequestID(r.Context()),
			)
		})
	}
}

// Recoverer recovers panics in downstream handlers, logs them with the
// request id (never leaking the stack to the client), and responds with the
// unified 500 error envelope. Mount it innermost so it wraps the actual
// handler directly.
//
// This only works cleanly if the panic happens before the handler writes
// its response: net/http has already flushed any bytes written via w.Write
// or a prior w.WriteHeader, so InternalError's own WriteHeader/body below
// becomes a no-op (logged by net/http, not fatal) and the client sees a
// truncated, not-well-formed body instead of the error envelope. Issue #5+
// domain handlers should build the full response (or at least decide the
// status code) before writing anything to w.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"request_id", GetRequestID(r.Context()),
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					InternalError(w, r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
