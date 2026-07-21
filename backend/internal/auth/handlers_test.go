package auth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func newTestRouter(t *testing.T, store *fakeStore) (chi.Router, *Handlers) {
	t.Helper()
	svc := &Service{Repo: store, JWTSecret: []byte("test-jwt-secret")}
	h := &Handlers{Service: svc, Log: slog.New(slog.NewTextHandler(bytesDiscard{}, nil))}
	r := chi.NewRouter()
	h.Mount(r, "test-bot-token", []byte("test-jwt-secret"))
	return r, h
}

type bytesDiscard struct{}

func (bytesDiscard) Write(p []byte) (int, error) { return len(p), nil }

func decodeEnvelope(t *testing.T, body []byte, v any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("invalid envelope JSON: %v, raw: %s", err, body)
	}
	if err := json.Unmarshal(envelope.Data, v); err != nil {
		t.Fatalf("invalid data payload: %v, raw: %s", err, envelope.Data)
	}
}

func TestHandlers_AdminLogin_Success(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	r, _ := newTestRouter(t, store)

	body := `{"email":"staff@example.com","password":"correct-password"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Tokens    tokenPairWire `json:"tokens"`
		AdminUser adminUserWire `json:"admin_user"`
	}
	decodeEnvelope(t, rec.Body.Bytes(), &got)
	if got.Tokens.AccessToken == "" || got.Tokens.RefreshToken == "" {
		t.Fatalf("expected both tokens, got %+v", got.Tokens)
	}
	if got.Tokens.TokenType != "Bearer" || got.Tokens.ExpiresIn != int(AccessTokenTTL.Seconds()) {
		t.Fatalf("unexpected token pair shape: %+v", got.Tokens)
	}
	if got.AdminUser.Email != "staff@example.com" || got.AdminUser.Role != "admin" {
		t.Fatalf("unexpected admin_user: %+v", got.AdminUser)
	}
}

func TestHandlers_AdminLogin_WrongPasswordReturns401(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	r, _ := newTestRouter(t, store)

	body := `{"email":"staff@example.com","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestHandlers_AdminLogin_ValidationErrorReturns422(t *testing.T) {
	store := newFakeStore()
	r, _ := newTestRouter(t, store)

	body := `{"email":"staff@example.com","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestHandlers_AdminLogin_MalformedJSONReturns400(t *testing.T) {
	store := newFakeStore()
	r, _ := newTestRouter(t, store)

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/login", bytes.NewBufferString(`{not json`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlers_FullSessionLifecycle(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	r, _ := newTestRouter(t, store)

	// 1. Login.
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/auth/login",
		bytes.NewBufferString(`{"email":"staff@example.com","password":"correct-password"}`))
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginBody struct {
		Tokens tokenPairWire `json:"tokens"`
	}
	decodeEnvelope(t, loginRec.Body.Bytes(), &loginBody)

	// 2. GET /admin/auth/me with the access token.
	meReq := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginBody.Tokens.AccessToken)
	meRec := httptest.NewRecorder()
	r.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d, body=%s", meRec.Code, meRec.Body.String())
	}

	// 3. Refresh rotates the pair.
	refreshReq := httptest.NewRequest(http.MethodPost, "/admin/auth/refresh",
		bytes.NewBufferString(`{"refresh_token":"`+loginBody.Tokens.RefreshToken+`"}`))
	refreshRec := httptest.NewRecorder()
	r.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	var refreshBody tokenPairWire
	decodeEnvelope(t, refreshRec.Body.Bytes(), &refreshBody)
	if refreshBody.RefreshToken == loginBody.Tokens.RefreshToken {
		t.Fatal("expected a rotated refresh token")
	}

	// 4. Reusing the pre-rotation refresh token must now fail.
	reuseReq := httptest.NewRequest(http.MethodPost, "/admin/auth/refresh",
		bytes.NewBufferString(`{"refresh_token":"`+loginBody.Tokens.RefreshToken+`"}`))
	reuseRec := httptest.NewRecorder()
	r.ServeHTTP(reuseRec, reuseReq)
	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token status = %d, want %d", reuseRec.Code, http.StatusUnauthorized)
	}

	// 5. Logout revokes the current (rotated) session.
	logoutReq := httptest.NewRequest(http.MethodPost, "/admin/auth/logout",
		bytes.NewBufferString(`{"refresh_token":"`+refreshBody.RefreshToken+`"}`))
	logoutReq.Header.Set("Authorization", "Bearer "+loginBody.Tokens.AccessToken)
	logoutRec := httptest.NewRecorder()
	r.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d, body=%s", logoutRec.Code, http.StatusNoContent, logoutRec.Body.String())
	}

	// 6. Refreshing with the now-logged-out token must be rejected.
	postLogoutReq := httptest.NewRequest(http.MethodPost, "/admin/auth/refresh",
		bytes.NewBufferString(`{"refresh_token":"`+refreshBody.RefreshToken+`"}`))
	postLogoutRec := httptest.NewRecorder()
	r.ServeHTTP(postLogoutRec, postLogoutReq)
	if postLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout refresh status = %d, want %d", postLogoutRec.Code, http.StatusUnauthorized)
	}
}

func TestHandlers_AdminMe_NoTokenReturns401(t *testing.T) {
	store := newFakeStore()
	r, _ := newTestRouter(t, store)

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlers_GetMe_InvalidInitDataReturns401(t *testing.T) {
	store := newFakeStore()
	r, _ := newTestRouter(t, store)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "tma not-a-valid-initdata")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestHandlers_GetMe_ValidInitDataUpsertsCustomer(t *testing.T) {
	store := newFakeStore()
	r, _ := newTestRouter(t, store)
	botToken := "test-bot-token"

	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(time.Now()),
		"user":      `{"id":777,"username":"trinity","first_name":"Trinity"}`,
	})

	req1 := httptest.NewRequest(http.MethodGet, "/me", nil)
	req1.Header.Set("Authorization", "tma "+raw)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first /me status = %d, body=%s", rec1.Code, rec1.Body.String())
	}
	var first customerWire
	decodeEnvelope(t, rec1.Body.Bytes(), &first)
	if first.TelegramID != 777 {
		t.Fatalf("unexpected customer: %+v", first)
	}

	// Second call with the same Telegram identity must return the same
	// customer id (upsert, not a duplicate insert).
	req2 := httptest.NewRequest(http.MethodGet, "/me", nil)
	req2.Header.Set("Authorization", "tma "+raw)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second /me status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var second customerWire
	decodeEnvelope(t, rec2.Body.Bytes(), &second)
	if second.ID != first.ID {
		t.Fatalf("expected the same customer id on repeat visit, got %d then %d", first.ID, second.ID)
	}
}

// TestHandlers_RoleGuardedRoute_WrongRoleForbidden demonstrates RBAC wired
// end-to-end (RequireAdminJWT + RequireRole composed, matching how a future
// domain router mounts a role-restricted admin operation). None of the five
// Auth Module operations wired in this issue require a specific role
// (docs/api/openapi.yaml: login/refresh are public, logout/me only require
// `adminJWT`), so this test mounts a synthetic route the same way a real
// admin operation would, to prove the composition itself behaves correctly.
func TestHandlers_RoleGuardedRoute_WrongRoleForbidden(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "manager@example.com", "correct-password", "content_manager", true)
	r, h := newTestRouter(t, store)

	r.Group(func(admin chi.Router) {
		admin.Use(RequireAdminJWT([]byte("test-jwt-secret"), h.Service.Repo))
		admin.Use(RequireRole("admin"))
		admin.Get("/admin/settings", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	loginReq := httptest.NewRequest(http.MethodPost, "/admin/auth/login",
		bytes.NewBufferString(`{"email":"manager@example.com","password":"correct-password"}`))
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginBody struct {
		Tokens tokenPairWire `json:"tokens"`
	}
	decodeEnvelope(t, loginRec.Body.Bytes(), &loginBody)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+loginBody.Tokens.AccessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (content_manager must not reach an admin-only route), body=%s",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}
}
