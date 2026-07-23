package auth

import (
	"errors"
	"testing"
	"time"
)

func newTestService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	return &Service{
		Repo:      store,
		JWTSecret: []byte("test-jwt-secret"),
		Now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func seedAdmin(t *testing.T, store *fakeStore, id int64, email, password, role string, active bool) *AdminRecord {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := &AdminRecord{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		RoleCode:     role,
		IsActive:     active,
		CreatedAt:    time.Now(),
	}
	store.addAdmin(a)
	return a
}

func TestService_Login_Success(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	pair, profile, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected both tokens to be issued, got %+v", pair)
	}
	if profile.Email != "staff@example.com" || profile.Role != "admin" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestService_Login_WrongPasswordRejected(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	_, _, err := svc.Login(t.Context(), "staff@example.com", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestService_Login_UnknownEmailRejected(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(t, store)

	_, _, err := svc.Login(t.Context(), "nobody@example.com", "whatever-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestService_Login_DeactivatedAccountRejected(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", false)
	svc := newTestService(t, store)

	_, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestService_Refresh_RotatesToken(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	pair, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := pair.RefreshToken

	newPair, err := svc.Refresh(t.Context(), oldRefresh)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if newPair.RefreshToken == oldRefresh {
		t.Fatal("expected rotation to produce a new refresh token")
	}

	// The newly issued refresh token must keep working across refreshes —
	// rotation must not cost the legitimate client its session.
	if _, err := svc.Refresh(t.Context(), newPair.RefreshToken); err != nil {
		t.Fatalf("Refresh with newly rotated token: %v", err)
	}
}

// TestService_Refresh_ReuseRevokesEverySession covers the replay case: a
// refresh token that was already rotated away is presented again. Only a
// stolen copy behaves that way — a well-behaved client always holds exactly
// one token — and there is no way to tell the thief's request from the
// victim's, so every session for that admin ends and both sides must log in
// again.
func TestService_Refresh_ReuseRevokesEverySession(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	pair, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := pair.RefreshToken

	newPair, err := svc.Refresh(t.Context(), oldRefresh)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Replaying the rotated token is rejected...
	if _, err := svc.Refresh(t.Context(), oldRefresh); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reusing rotated token: err = %v, want ErrInvalidRefreshToken", err)
	}

	// ...and takes the session that replaced it down with it.
	if _, err := svc.Refresh(t.Context(), newPair.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("session surviving a replay: err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Refresh_UnknownTokenRejected(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(t, store)

	if _, err := svc.Refresh(t.Context(), "never-issued-token"); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Refresh_ExpiredTokenRejected(t *testing.T) {
	store := newFakeStore()
	admin := seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)

	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if _, err := store.InsertRefreshToken(t.Context(), admin.ID, hash, time.Unix(1_600_000_000, 0)); err != nil {
		t.Fatalf("InsertRefreshToken: %v", err)
	}

	svc := newTestService(t, store) // svc.now() = 1_700_000_000, well after the token's expiry
	if _, err := svc.Refresh(t.Context(), raw); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Refresh_RevokedTokenRejected(t *testing.T) {
	store := newFakeStore()
	admin := seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	id, err := store.InsertRefreshToken(t.Context(), admin.ID, hash, time.Now().Add(RefreshTokenTTL))
	if err != nil {
		t.Fatalf("InsertRefreshToken: %v", err)
	}
	if err := store.RevokeRefreshToken(t.Context(), id); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	if _, err := svc.Refresh(t.Context(), raw); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Logout_RevokesSpecificToken(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	pair, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.Logout(t.Context(), 1, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, err := svc.Refresh(t.Context(), pair.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh after logout: err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Logout_EmptyTokenRevokesAllSessions(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "admin", true)
	svc := newTestService(t, store)

	pair1, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	pair2, _, err := svc.Login(t.Context(), "staff@example.com", "correct-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.Logout(t.Context(), 1, ""); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, err := svc.Refresh(t.Context(), pair1.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("pair1 refresh after logout-all: err = %v, want ErrInvalidRefreshToken", err)
	}
	if _, err := svc.Refresh(t.Context(), pair2.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("pair2 refresh after logout-all: err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestService_Me_ReturnsProfile(t *testing.T) {
	store := newFakeStore()
	seedAdmin(t, store, 1, "staff@example.com", "correct-password", "content_manager", true)
	svc := newTestService(t, store)

	profile, err := svc.Me(t.Context(), 1)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if profile.Role != "content_manager" {
		t.Fatalf("unexpected role: %s", profile.Role)
	}
}

func TestService_MeCustomer_UpsertsByTelegramID(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(t, store)

	first, err := svc.MeCustomer(t.Context(), TelegramUser{ID: 555, Username: "neo", FirstName: "Thomas"})
	if err != nil {
		t.Fatalf("MeCustomer (insert): %v", err)
	}
	if first.TelegramID != 555 {
		t.Fatalf("unexpected customer: %+v", first)
	}

	second, err := svc.MeCustomer(t.Context(), TelegramUser{ID: 555, Username: "neo", FirstName: "Thomas", LastName: "Anderson"})
	if err != nil {
		t.Fatalf("MeCustomer (update): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same customer id on repeat visit, got %d then %d", first.ID, second.ID)
	}
	if second.LastName == nil || *second.LastName != "Anderson" {
		t.Fatalf("expected profile fields to refresh from Telegram, got %+v", second)
	}
}
