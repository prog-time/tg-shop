package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- RequireInitData ---------------------------------------------------

func TestRequireInitData_ValidSignaturePassesThrough(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	// RequireInitData always checks freshness against the real wall clock
	// (auth.go calls VerifyInitData with time.Now(), not an injectable
	// clock), so the fixture's auth_date must be real "now", not a fixed
	// timestamp — unlike VerifyInitData's own direct unit tests in
	// initdata_test.go, which control both sides of the comparison.
	now := time.Now()
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42,"username":"neo","first_name":"Thomas","last_name":"Anderson"}`,
	})

	var gotUser TelegramUser
	var gotOK bool
	h := RequireInitData(botToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotOK = TelegramUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "tma "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !gotOK {
		t.Fatal("expected a TelegramUser to be attached to the request context")
	}
	if gotUser.ID != 42 || gotUser.Username != "neo" {
		t.Fatalf("unexpected TelegramUser: %+v", gotUser)
	}
}

func TestRequireInitData_MissingHeaderRejected(t *testing.T) {
	called := false
	h := RequireInitData("bot-token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler must not run without a valid Authorization header")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireInitData_TamperedHashRejected(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(now),
		"user":      `{"id":42}`,
	})
	tampered := flipLastHexChar(t, raw)

	called := false
	h := RequireInitData(botToken)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "tma "+tampered)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler must not run with a tampered initData signature")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireInitData_StaleAuthDateRejected(t *testing.T) {
	botToken := "123456:ABC-DEF-test-token"
	now := time.Unix(1_700_000_000, 0)
	stale := now.Add(-InitDataMaxAge - time.Hour)
	raw := buildSignedInitData(t, botToken, map[string]string{
		"auth_date": unixString(stale),
		"user":      `{"id":42}`,
	})

	called := false
	h := RequireInitData(botToken)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "tma "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler must not run with a stale auth_date")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- RequireAdminJWT -----------------------------------------------------

type fakeAdminLoader struct {
	admins map[int64]*AdminRecord
}

func (f *fakeAdminLoader) GetAdminByID(_ context.Context, id int64) (*AdminRecord, error) {
	a, ok := f.admins[id]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

func TestRequireAdminJWT_ValidTokenAttachesIdentity(t *testing.T) {
	secret := []byte("test-jwt-secret")
	loader := &fakeAdminLoader{admins: map[int64]*AdminRecord{
		7: {ID: 7, Email: "staff@example.com", RoleCode: "admin", IsActive: true},
	}}
	token, err := IssueAccessToken(secret, 7, "admin", time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	var gotIdentity AdminIdentity
	var gotOK bool
	h := RequireAdminJWT(secret, loader)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, gotOK = AdminIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !gotOK || gotIdentity.ID != 7 || gotIdentity.Role != "admin" {
		t.Fatalf("unexpected identity: ok=%v %+v", gotOK, gotIdentity)
	}
}

func TestRequireAdminJWT_MissingHeaderRejected(t *testing.T) {
	h := RequireAdminJWT([]byte("secret"), &fakeAdminLoader{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminJWT_BadSignatureRejected(t *testing.T) {
	token, err := IssueAccessToken([]byte("other-secret"), 7, "admin", time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	h := RequireAdminJWT([]byte("secret"), &fakeAdminLoader{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminJWT_DeactivatedAdminRejected(t *testing.T) {
	secret := []byte("test-jwt-secret")
	loader := &fakeAdminLoader{admins: map[int64]*AdminRecord{
		7: {ID: 7, Email: "staff@example.com", RoleCode: "admin", IsActive: false},
	}}
	token, err := IssueAccessToken(secret, 7, "admin", time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	h := RequireAdminJWT(secret, loader)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not run for a deactivated admin")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- RequireRole -----------------------------------------------------------

func TestRequireRole_AllowedRolePasses(t *testing.T) {
	called := false
	h := RequireRole("admin", "order_manager")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/orders", nil)
	ctx := WithAdminIdentity(req.Context(), AdminIdentity{ID: 1, Role: "order_manager"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(ctx))

	if !called {
		t.Fatal("expected the wrapped handler to run for an allowed role")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireRole_DisallowedRoleForbidden(t *testing.T) {
	called := false
	h := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/orders", nil)
	ctx := WithAdminIdentity(req.Context(), AdminIdentity{ID: 1, Role: "content_manager"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(ctx))

	if called {
		t.Fatal("handler must not run for a disallowed role")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireRole_NoIdentityFailsClosed(t *testing.T) {
	h := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not run without an AdminIdentity in context")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/orders", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
