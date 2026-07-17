// Package auth is the home of the Auth Module (docs/architecture.md, Level 3
// — Components of Backend API): verifying the storefront's Telegram Web App
// `initData` and the admin panel's JWT. Both attachment points below are
// placeholder pass-throughs — real verification is issue #5+.
package auth

import "net/http"

// RequireInitData will verify the Telegram Web App `initData` signature (the
// contract's `initData` security scheme, docs/api/openapi.yaml) and attach
// the resolved customer identity to the request context. Per
// docs/architecture.md's Security rules, verification must happen only here,
// server-side — never on the client. Domain routers for storefront routes
// mount this middleware once it verifies.
//
// TODO(#5): verify initData against the bot token; currently a no-op
// pass-through, so every request is treated as unauthenticated.
func RequireInitData(next http.Handler) http.Handler {
	return next
}

// RequireAdminJWT will verify the staff JWT issued by `POST
// /admin/auth/login` (the contract's `adminJWT` security scheme) and attach
// the resolved admin identity and role (for RBAC) to the request context.
// Domain routers for admin routes mount this middleware once it verifies.
//
// TODO(#5+): verify the admin JWT and apply RBAC; currently a no-op
// pass-through, so every request is treated as unauthenticated.
func RequireAdminJWT(next http.Handler) http.Handler {
	return next
}
