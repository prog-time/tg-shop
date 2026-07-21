// Package auth is the home of the Auth Module (docs/architecture.md, Level 3
// — Components of Backend API): verifying the storefront's Telegram Web App
// `initData` and the admin panel's JWT, plus the staff login/refresh/logout
// session lifecycle. It is the single place either mechanism is
// implemented — no other package parses initData, mints a JWT, or reads
// admin_refresh_tokens.
package auth

import (
	"net/http"
	"time"

	"github.com/prog-time/tg-shop/backend/internal/httpx"
)

// RequireInitData verifies the Telegram Web App `initData` signature sent as
// `Authorization: tma <initData>` (the contract's `initData` security
// scheme, docs/api/openapi.yaml) against botToken, per Telegram's documented
// algorithm (see VerifyInitData), and attaches the resolved TelegramUser to
// the request context (TelegramUserFromContext). Per
// docs/architecture.md's Security rules, verification happens only here,
// server-side — never on the client.
//
// Any failure — missing header, wrong scheme, tampered signature, stale
// auth_date — yields the same uniform 401; the specific reason is never
// distinguished for the client.
func RequireInitData(botToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := extractTMA(r.Header.Get("Authorization"))
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "missing or malformed Authorization header")
				return
			}

			user, err := VerifyInitData(botToken, raw, InitDataMaxAge, time.Now())
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid initData")
				return
			}

			ctx := WithTelegramUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractTMA pulls the raw initData string out of an `Authorization: tma
// <initData>` header. The scheme keyword is matched case-insensitively
// (common for HTTP auth schemes) but is otherwise required verbatim.
func extractTMA(header string) (string, bool) {
	const prefix = "tma "
	if len(header) <= len(prefix) {
		return "", false
	}
	if !equalFoldASCII(header[:len(prefix)], prefix) {
		return "", false
	}
	return header[len(prefix):], true
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// RequireAdminJWT verifies the staff access JWT issued by `POST
// /admin/auth/login` (the contract's `adminJWT` security scheme) via
// ParseAccessToken, then reloads the admin account from PostgreSQL by the
// token's admin_id and attaches the resulting AdminIdentity to the request
// context (AdminIdentityFromContext).
//
// The reload (rather than trusting the JWT's embedded role claim) is
// deliberate: it is the only way a deactivated admin
// (admin_users.is_active = false) or a role change takes effect before the
// short-lived (15m) access token would otherwise expire on its own.
// Domain routers needing a specific role on top of "is an authenticated
// admin" compose this with RequireRole.
func RequireAdminJWT(secret []byte, repo AdminByIDLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "missing or malformed Authorization header")
				return
			}

			claims, err := ParseAccessToken(secret, tokenString)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid or expired access token")
				return
			}

			admin, err := repo.GetAdminByID(r.Context(), claims.AdminID)
			if err != nil || !admin.IsActive {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid or expired access token")
				return
			}

			ctx := WithAdminIdentity(r.Context(), AdminIdentity{
				ID:       admin.ID,
				Email:    admin.Email,
				Role:     admin.RoleCode,
				FullName: admin.FullName,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) {
		return "", false
	}
	if !equalFoldASCII(header[:len(prefix)], prefix) {
		return "", false
	}
	return header[len(prefix):], true
}

// RequireRole guards a route behind RBAC: it must be mounted after
// RequireAdminJWT (it reads the AdminIdentity that middleware attaches) and
// responds 403 Forbidden if the authenticated admin's role isn't one of
// allowed. Domain routers use this per-operation, matching the role(s)
// documented in each admin operation's description in
// docs/api/openapi.yaml (e.g. "Required role: `content_manager` or
// `admin`.").
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		allowedSet[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := AdminIdentityFromContext(r.Context())
			if !ok {
				// Programmer error (RequireRole mounted without
				// RequireAdminJWT ahead of it) — fail closed rather than
				// letting an unauthenticated request through.
				httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "authentication required")
				return
			}
			if _, ok := allowedSet[identity.Role]; !ok {
				httpx.WriteError(w, http.StatusForbidden, httpx.ErrCodeForbidden, "your role does not have access to this operation")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
