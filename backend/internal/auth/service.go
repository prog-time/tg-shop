package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidCredentials is returned by Service.Login for every failure mode
// — unknown email, wrong password, and (deliberately) a deactivated account
// all collapse into this single sentinel. Per the brief ("Login must not
// leak whether the email exists"), the HTTP layer maps it to one uniform
// 401 message regardless of which of those it was.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrInvalidRefreshToken covers every refresh/logout lookup failure: token
// not found, already revoked, or expired.
var ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")

// TokenPair is the access/refresh pair minted by Login and Refresh.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
}

// AdminProfile is the staff account shape Service returns to the HTTP layer
// (kept independent of the generated openapi.AdminUser so this package has
// no import-cycle-prone dependency on internal/openapi; handlers.go does
// the one-line translation).
type AdminProfile struct {
	ID        int64
	Email     string
	FullName  *string
	Role      string
	IsActive  bool
	CreatedAt time.Time
}

// CustomerProfile is the storefront customer shape Service returns.
type CustomerProfile struct {
	ID         int64
	TelegramID int64
	Username   *string
	FirstName  *string
	LastName   *string
	CreatedAt  time.Time
}

// Service implements the Auth Module's business logic: staff
// login/refresh/logout/me and the storefront's initData-derived `/me`.
// HTTP decoding/encoding lives in handlers.go; Service never touches
// net/http.
type Service struct {
	Repo      Store
	JWTSecret []byte

	// Now defaults to time.Now; overridable in tests for exact TTL/rotation
	// assertions.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Login exchanges email/password for a new access/refresh token pair.
// Every failure path — unknown email, wrong password, inactive account —
// returns ErrInvalidCredentials, and Login always performs an argon2id
// verify (against a fixed dummy hash when the account doesn't exist) so the
// unknown-email and wrong-password paths take comparable time; neither
// leaks which one occurred.
func (s *Service) Login(ctx context.Context, email, password string) (*TokenPair, *AdminProfile, error) {
	email = normalizeEmail(email)

	admin, err := s.Repo.GetAdminByEmail(ctx, email)
	found := err == nil
	hashToCheck := dummyPasswordHash
	if found {
		hashToCheck = admin.PasswordHash
	} else if !errors.Is(err, ErrNotFound) {
		return nil, nil, fmt.Errorf("lookup admin by email: %w", err)
	}

	ok, verifyErr := VerifyPassword(password, hashToCheck)
	if verifyErr != nil && found {
		// A malformed hash for a *real* account is an operational problem,
		// not a client error — surface it as internal rather than folding
		// it into "invalid credentials".
		return nil, nil, fmt.Errorf("verify password: %w", verifyErr)
	}
	if !found || !ok || !admin.IsActive {
		return nil, nil, ErrInvalidCredentials
	}

	pair, err := s.issueTokenPair(ctx, admin.ID, admin.RoleCode)
	if err != nil {
		return nil, nil, err
	}
	return pair, toAdminProfile(admin), nil
}

// Refresh exchanges a valid, non-revoked, non-expired refresh token for a
// brand new access/refresh pair, revoking the old refresh token in the same
// transaction (rotation). Rotation — rather than reusing the same refresh
// token across accesses — is the safer default: it bounds the blast radius
// of a leaked refresh token to a single use before the legitimate client's
// next refresh silently invalidates it, which also makes reuse of a stolen
// token detectable (a second "refresh" of an already-rotated token fails).
func (s *Service) Refresh(ctx context.Context, rawToken string) (*TokenPair, error) {
	if rawToken == "" {
		return nil, ErrInvalidRefreshToken
	}
	hash := HashRefreshToken(rawToken)

	record, err := s.Repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("lookup refresh token: %w", err)
	}
	if record.RevokedAt != nil || !record.ExpiresAt.After(s.now()) {
		return nil, ErrInvalidRefreshToken
	}

	admin, err := s.Repo.GetAdminByID(ctx, record.AdminUserID)
	if err != nil || !admin.IsActive {
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("lookup admin by id: %w", err)
		}
		return nil, ErrInvalidRefreshToken
	}

	newRaw, newHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	newExpiresAt := s.now().Add(RefreshTokenTTL)

	if _, err := s.Repo.RotateRefreshToken(ctx, record.ID, admin.ID, newHash, newExpiresAt); err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	accessToken, err := IssueAccessToken(s.JWTSecret, admin.ID, admin.RoleCode, s.now())
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRaw,
		TokenType:    "Bearer",
		ExpiresIn:    int(AccessTokenTTL.Seconds()),
	}, nil
}

// Logout revokes sessions for adminID. If rawToken is non-empty, only the
// session it belongs to is revoked (and only if it does belong to adminID —
// a token hash under a different admin's session is left untouched rather
// than silently succeeding as if it mattered). If rawToken is empty (the
// contract makes the request body optional), every active session for
// adminID is revoked — the safer reading of "log out" absent a specific
// token to target.
//
// Logout never errors on "already logged out" — revoking an unknown,
// already-revoked, or foreign token is treated the same as revoking a valid
// one: silently a no-op, so logout stays idempotent and never leaks session
// existence.
func (s *Service) Logout(ctx context.Context, adminID int64, rawToken string) error {
	if rawToken == "" {
		return s.Repo.RevokeAllRefreshTokens(ctx, adminID)
	}

	record, err := s.Repo.GetRefreshTokenByHash(ctx, HashRefreshToken(rawToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("lookup refresh token: %w", err)
	}
	if record.AdminUserID != adminID {
		return nil
	}
	return s.Repo.RevokeRefreshToken(ctx, record.ID)
}

// Me reloads the current staff session's admin account (fresh from the DB,
// same as RequireAdminJWT already did, but Me is the one place the contract
// requires it as a response body rather than just gate access).
func (s *Service) Me(ctx context.Context, adminID int64) (*AdminProfile, error) {
	admin, err := s.Repo.GetAdminByID(ctx, adminID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("lookup admin by id: %w", err)
	}
	return toAdminProfile(admin), nil
}

// MeCustomer upserts and returns the storefront customer for a verified
// Telegram user (see RequireInitData/VerifyInitData — u is only ever
// constructed there).
func (s *Service) MeCustomer(ctx context.Context, u TelegramUser) (*CustomerProfile, error) {
	c, err := s.Repo.UpsertCustomerByTelegramID(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("upsert customer: %w", err)
	}
	return &CustomerProfile{
		ID:         c.ID,
		TelegramID: c.TelegramID,
		Username:   c.Username,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		CreatedAt:  c.CreatedAt,
	}, nil
}

func (s *Service) issueTokenPair(ctx context.Context, adminID int64, role string) (*TokenPair, error) {
	rawRefresh, hash, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	if _, err := s.Repo.InsertRefreshToken(ctx, adminID, hash, s.now().Add(RefreshTokenTTL)); err != nil {
		return nil, fmt.Errorf("insert refresh token: %w", err)
	}

	accessToken, err := IssueAccessToken(s.JWTSecret, adminID, role, s.now())
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(AccessTokenTTL.Seconds()),
	}, nil
}

func toAdminProfile(a *AdminRecord) *AdminProfile {
	return &AdminProfile{
		ID:        a.ID,
		Email:     a.Email,
		FullName:  a.FullName,
		Role:      a.RoleCode,
		IsActive:  a.IsActive,
		CreatedAt: a.CreatedAt,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
