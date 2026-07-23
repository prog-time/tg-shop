package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/prog-time/tg-shop/backend/internal/httpx"
)

// Handlers is the HTTP layer for the Auth Module's five operations (the
// `auth` tag in docs/api/openapi.yaml): decoding requests and encoding
// responses to match the contract's wire shapes and error envelope exactly.
// All business logic lives in Service; Handlers never touches PostgreSQL
// directly.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

// log returns h.Log, or the default logger when it was left unset. Every
// call site is on an error path, so a nil Log would panic precisely when
// something has already gone wrong — turning a recoverable 500 into a lost
// request and an unlogged cause.
func (h *Handlers) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

// Mount registers every Auth Module route on r, applying the middleware each
// operation's security scheme requires (docs/api/openapi.yaml):
//
//   - POST /admin/auth/login, POST /admin/auth/refresh — `security: []`,
//     public: these endpoints authenticate the caller themselves.
//   - POST /admin/auth/logout, GET /admin/auth/me — `adminJWT`.
//   - GET /me — the top-level default `initData` (storefront).
//
// botToken and jwtSecret parametrize RequireInitData/RequireAdminJWT;
// RequireAdminJWT reloads the admin account through h.Service.Repo (the same
// Store the service uses), so a deactivation is enforced identically whether
// checked by the middleware or by Service.Me.
func (h *Handlers) Mount(r chi.Router, botToken string, jwtSecret []byte) {
	r.Post("/admin/auth/login", h.AdminLogin)
	r.Post("/admin/auth/refresh", h.AdminRefresh)

	r.Group(func(admin chi.Router) {
		admin.Use(RequireAdminJWT(jwtSecret, h.Service.Repo))
		admin.Post("/admin/auth/logout", h.AdminLogout)
		admin.Get("/admin/auth/me", h.GetAdminMe)
	})

	r.Group(func(storefront chi.Router) {
		storefront.Use(RequireInitData(botToken))
		storefront.Get("/me", h.GetMe)
	})
}

// --- wire shapes -----------------------------------------------------------
//
// These mirror the generated types in internal/openapi/openapi.gen.go
// (AdminLoginRequest, RefreshRequest, TokenPair, AdminUser, Customer)
// field-for-field. They are re-declared here, rather than importing
// internal/openapi, to keep this package's only dependency direction intact
// (openapi depends on nothing of this package's own types leaking out) and
// because Service already returns its own independent shapes (AdminProfile,
// CustomerProfile, TokenPair) — see service.go's doc comment.

type adminLoginRequestWire struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequestWire struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenPairWire struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type adminUserWire struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	FullName  *string   `json:"full_name,omitempty"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type customerWire struct {
	ID         int64     `json:"id"`
	TelegramID int64     `json:"telegram_id"`
	Username   *string   `json:"username,omitempty"`
	FirstName  *string   `json:"first_name,omitempty"`
	LastName   *string   `json:"last_name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type adminLoginDataWire struct {
	Tokens    tokenPairWire `json:"tokens"`
	AdminUser adminUserWire `json:"admin_user"`
}

func toTokenPairWire(p *TokenPair) tokenPairWire {
	return tokenPairWire{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		TokenType:    p.TokenType,
		ExpiresIn:    p.ExpiresIn,
	}
}

func toAdminUserWire(p *AdminProfile) adminUserWire {
	return adminUserWire{
		ID:        p.ID,
		Email:     p.Email,
		FullName:  p.FullName,
		Role:      p.Role,
		IsActive:  p.IsActive,
		CreatedAt: p.CreatedAt,
	}
}

func toCustomerWire(p *CustomerProfile) customerWire {
	return customerWire{
		ID:         p.ID,
		TelegramID: p.TelegramID,
		Username:   p.Username,
		FirstName:  p.FirstName,
		LastName:   p.LastName,
		CreatedAt:  p.CreatedAt,
	}
}

// decodeJSONBody decodes body into v. An empty body is treated as a
// zero-valued v with no error (several operations here have an optional
// request body, per the contract's `requestBody.required: false`); any other
// decode failure reports ok=false after writing the contract's 400
// BadRequest envelope ("malformed request... invalid JSON").
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) (ok bool) {
	if r.Body == nil {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		httpx.WriteError(w, http.StatusBadRequest, httpx.ErrCodeValidation, "invalid JSON body")
		return false
	}
	return true
}

// --- handlers ---------------------------------------------------------------

// AdminLogin implements POST /admin/auth/login.
func (h *Handlers) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var req adminLoginRequestWire
	if !decodeJSONBody(w, r, &req) {
		return
	}

	var details []httpx.ErrorDetail
	if strings.TrimSpace(req.Email) == "" || !strings.Contains(req.Email, "@") {
		details = append(details, httpx.ErrorDetail{Field: "email", Issue: "must be a valid email address"})
	}
	if len(req.Password) < 8 {
		details = append(details, httpx.ErrorDetail{Field: "password", Issue: "must be at least 8 characters"})
	}
	if len(details) > 0 {
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.ErrCodeValidation, "validation failed", details...)
		return
	}

	pair, profile, err := h.Service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid email or password")
			return
		}
		h.log().Error("admin login failed", "err", err)
		httpx.InternalError(w, r)
		return
	}

	httpx.WriteData(w, http.StatusOK, adminLoginDataWire{
		Tokens:    toTokenPairWire(pair),
		AdminUser: toAdminUserWire(profile),
	})
}

// AdminRefresh implements POST /admin/auth/refresh.
func (h *Handlers) AdminRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequestWire
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.ErrCodeValidation, "validation failed",
			httpx.ErrorDetail{Field: "refresh_token", Issue: "is required"})
		return
	}

	pair, err := h.Service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid or expired refresh token")
			return
		}
		h.log().Error("admin refresh failed", "err", err)
		httpx.InternalError(w, r)
		return
	}

	httpx.WriteData(w, http.StatusOK, toTokenPairWire(pair))
}

// AdminLogout implements POST /admin/auth/logout. Requires RequireAdminJWT.
func (h *Handlers) AdminLogout(w http.ResponseWriter, r *http.Request) {
	identity, ok := AdminIdentityFromContext(r.Context())
	if !ok {
		// Programmer error (mounted without RequireAdminJWT); fail closed.
		httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "authentication required")
		return
	}

	var req refreshRequestWire
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.Service.Logout(r.Context(), identity.ID, req.RefreshToken); err != nil {
		h.log().Error("admin logout failed", "err", err)
		httpx.InternalError(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAdminMe implements GET /admin/auth/me. Requires RequireAdminJWT.
func (h *Handlers) GetAdminMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := AdminIdentityFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "authentication required")
		return
	}

	profile, err := h.Service.Me(r.Context(), identity.ID)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "invalid or expired access token")
			return
		}
		h.log().Error("admin me failed", "err", err)
		httpx.InternalError(w, r)
		return
	}

	httpx.WriteData(w, http.StatusOK, toAdminUserWire(profile))
}

// GetMe implements GET /me (storefront). Requires RequireInitData.
func (h *Handlers) GetMe(w http.ResponseWriter, r *http.Request) {
	tgUser, ok := TelegramUserFromContext(r.Context())
	if !ok {
		// Programmer error (mounted without RequireInitData); fail closed.
		httpx.WriteError(w, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "authentication required")
		return
	}

	profile, err := h.Service.MeCustomer(r.Context(), tgUser)
	if err != nil {
		h.log().Error("get me failed", "err", err)
		httpx.InternalError(w, r)
		return
	}

	httpx.WriteData(w, http.StatusOK, toCustomerWire(profile))
}
