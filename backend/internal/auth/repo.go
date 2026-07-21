package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by repository lookups that find no matching row.
// It is never surfaced to an HTTP client directly — callers translate it
// into the uniform, non-leaking errors Service returns (see service.go).
var ErrNotFound = errors.New("auth: not found")

// AdminByIDLoader is the minimal capability RequireAdminJWT needs: reload an
// admin account fresh from storage on every request (see the middleware's
// own doc comment in auth.go for why a cached/JWT-only role claim isn't
// enough). *Repo satisfies this in production; unit tests substitute a small
// in-memory fake so the middleware can be exercised without a live Postgres
// connection.
type AdminByIDLoader interface {
	GetAdminByID(ctx context.Context, id int64) (*AdminRecord, error)
}

// Store is the full persistence contract Service depends on. *Repo
// (below) is the production implementation backed by pgx; tests substitute
// an in-memory fake implementing the same interface so business-logic
// coverage (login, refresh rotation, logout, uniform error handling) doesn't
// require a live database. The SQL in *Repo's methods is still exercised
// against real Postgres separately (see repo_integration_test.go).
type Store interface {
	AdminByIDLoader
	GetAdminByEmail(ctx context.Context, email string) (*AdminRecord, error)
	InsertRefreshToken(ctx context.Context, adminUserID int64, tokenHash string, expiresAt time.Time) (int64, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshTokenRecord, error)
	RevokeRefreshToken(ctx context.Context, id int64) error
	RevokeAllRefreshTokens(ctx context.Context, adminUserID int64) error
	RotateRefreshToken(ctx context.Context, oldID, adminUserID int64, newTokenHash string, newExpiresAt time.Time) (int64, error)
	UpsertCustomerByTelegramID(ctx context.Context, u TelegramUser) (*CustomerRecord, error)
}

// var assertion: *Repo must keep satisfying Store as it evolves.
var _ Store = (*Repo)(nil)

// AdminRecord is the admin_users row joined with its role code, as needed by
// login, JWT issuance, and RBAC.
type AdminRecord struct {
	ID           int64
	Email        string
	PasswordHash string
	RoleCode     string
	IsActive     bool
	FullName     *string
	CreatedAt    time.Time
}

// RefreshTokenRecord is one admin_refresh_tokens row.
type RefreshTokenRecord struct {
	ID          int64
	AdminUserID int64
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

// CustomerRecord is a users row (the storefront customer identity).
type CustomerRecord struct {
	ID         int64
	TelegramID int64
	Username   *string
	FirstName  *string
	LastName   *string
	CreatedAt  time.Time
}

// Repo is the pgx-backed persistence for the Auth Module. Per ADR-005/the
// project's no-ORM rule, it is hand-written SQL over pgxpool — no query
// builder, no generated repository.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo builds a Repo over an already-connected pool (api owns the pool's
// lifecycle; see cmd/api/main.go).
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const adminSelectCols = `au.id, au.email, au.password_hash, r.code, au.is_active, au.full_name, au.created_at`

func scanAdmin(row pgx.Row) (*AdminRecord, error) {
	var a AdminRecord
	if err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.RoleCode, &a.IsActive, &a.FullName, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan admin: %w", err)
	}
	return &a, nil
}

// GetAdminByEmail looks up a staff account by email (case-sensitive; emails
// are normalized to lowercase at write time — out of scope here since
// admin-user creation is a different module — so callers should lowercase
// before calling, which Service.Login does).
func (r *Repo) GetAdminByEmail(ctx context.Context, email string) (*AdminRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+adminSelectCols+`
		FROM admin_users au
		JOIN roles r ON r.id = au.role_id
		WHERE au.email = $1
	`, email)
	return scanAdmin(row)
}

// GetAdminByID reloads a staff account by id. Called on every admin-JWT
// request so a deactivation or role change is enforced immediately rather
// than only after the access token expires.
func (r *Repo) GetAdminByID(ctx context.Context, id int64) (*AdminRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+adminSelectCols+`
		FROM admin_users au
		JOIN roles r ON r.id = au.role_id
		WHERE au.id = $1
	`, id)
	return scanAdmin(row)
}

// InsertRefreshToken persists a new session row. tokenHash is
// HashRefreshToken(raw) — the raw token itself is never stored, per the
// admin_refresh_tokens migration's own rule.
func (r *Repo) InsertRefreshToken(ctx context.Context, adminUserID int64, tokenHash string, expiresAt time.Time) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO admin_refresh_tokens (admin_user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`, adminUserID, tokenHash, expiresAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert refresh token: %w", err)
	}
	return id, nil
}

// GetActiveRefreshTokenByHash finds a non-revoked, non-expired session by
// its token hash. It intentionally does NOT filter revoked_at/expires_at in
// SQL (beyond what's needed for the index) so callers can distinguish
// "doesn't exist" from "exists but revoked/expired" for logging, while still
// returning a uniform error to the HTTP client either way.
func (r *Repo) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshTokenRecord, error) {
	var t RefreshTokenRecord
	err := r.pool.QueryRow(ctx, `
		SELECT id, admin_user_id, expires_at, revoked_at
		FROM admin_refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&t.ID, &t.AdminUserID, &t.ExpiresAt, &t.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return &t, nil
}

// RevokeRefreshToken sets revoked_at = now() for the given session id.
// Idempotent: revoking an already-revoked row is a no-op (revoked_at is
// left at its original value, never bumped forward), matching the
// migration's "logout must genuinely revoke, never DELETE" rule.
func (r *Repo) RevokeRefreshToken(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_refresh_tokens SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokens revokes every active session for adminUserID. Used
// by logout when no specific refresh_token is presented in the request body
// (the contract makes the body optional) — the safer reading of "log this
// staff member out" is "end every session", not "do nothing".
func (r *Repo) RevokeAllRefreshTokens(ctx context.Context, adminUserID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_refresh_tokens SET revoked_at = now()
		WHERE admin_user_id = $1 AND revoked_at IS NULL
	`, adminUserID)
	if err != nil {
		return fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically revokes oldID and inserts a new session row
// in one transaction, so a crash between the two can never leave both the
// old and new tokens simultaneously valid (or both invalid).
func (r *Repo) RotateRefreshToken(ctx context.Context, oldID, adminUserID int64, newTokenHash string, newExpiresAt time.Time) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE admin_refresh_tokens SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL
	`, oldID); err != nil {
		return 0, fmt.Errorf("revoke old refresh token: %w", err)
	}

	var newID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin_refresh_tokens (admin_user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`, adminUserID, newTokenHash, newExpiresAt).Scan(&newID); err != nil {
		return 0, fmt.Errorf("insert rotated refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return newID, nil
}

// UpsertCustomerByTelegramID creates or refreshes the customer row for a
// verified Telegram user. Profile fields (username/first_name/last_name)
// are overwritten on every call — a judgment call: Telegram accounts change
// these, and there is no separate "update profile" flow, so `/me` is the
// only place they can ever be kept in sync with Telegram.
func (r *Repo) UpsertCustomerByTelegramID(ctx context.Context, u TelegramUser) (*CustomerRecord, error) {
	var c CustomerRecord
	var username, firstName, lastName *string
	if u.Username != "" {
		username = &u.Username
	}
	if u.FirstName != "" {
		firstName = &u.FirstName
	}
	if u.LastName != "" {
		lastName = &u.LastName
	}

	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name
		RETURNING id, telegram_id, username, first_name, last_name, created_at
	`, u.ID, username, firstName, lastName).Scan(
		&c.ID, &c.TelegramID, &c.Username, &c.FirstName, &c.LastName, &c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert customer: %w", err)
	}
	return &c, nil
}
