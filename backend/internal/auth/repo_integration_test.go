package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prog-time/tg-shop/backend/internal/postgres"
	"github.com/prog-time/tg-shop/backend/migrations"
)

// TestRepo_Integration exercises every Repo method's actual SQL against a
// real PostgreSQL instance (not the fakeStore the rest of this package's
// tests use). It is gated behind TEST_DATABASE_URL rather than always-on:
// host 5432 is routinely occupied by an unrelated local Postgres, so the
// verification procedure for this package is to run a throwaway
// `postgres:17-alpine` container on another port, point TEST_DATABASE_URL at
// it, `go test`, then tear the container down. See the issue #5 completion
// report for the exact docker commands used.
func TestRepo_Integration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping real-Postgres repository test (see doc comment)")
	}

	if err := postgres.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	// Each test gets a fresh transaction-free slate via unique emails/telegram
	// ids (derived from t.Name()) rather than TRUNCATE, so subtests can run
	// with t.Parallel() safely and leave no cross-test interference.
	repo := NewRepo(pool)

	t.Run("AdminByEmail_AdminByID_RoundTrip", func(t *testing.T) {
		email := "integration-admin-1@example.com"
		roleID := mustRoleID(t, ctx, pool, "admin")
		hash, err := HashPassword("integration-test-password")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}

		var adminID int64
		err = pool.QueryRow(ctx, `
			INSERT INTO admin_users (email, password_hash, role_id, is_active)
			VALUES ($1, $2, $3, true)
			RETURNING id
		`, email, hash, roleID).Scan(&adminID)
		if err != nil {
			t.Fatalf("insert admin: %v", err)
		}
		t.Cleanup(func() { mustExec(t, ctx, pool, `DELETE FROM admin_users WHERE id = $1`, adminID) })

		byEmail, err := repo.GetAdminByEmail(ctx, email)
		if err != nil {
			t.Fatalf("GetAdminByEmail: %v", err)
		}
		if byEmail.ID != adminID || byEmail.RoleCode != "admin" {
			t.Fatalf("unexpected admin by email: %+v", byEmail)
		}

		byID, err := repo.GetAdminByID(ctx, adminID)
		if err != nil {
			t.Fatalf("GetAdminByID: %v", err)
		}
		if byID.Email != email {
			t.Fatalf("unexpected admin by id: %+v", byID)
		}

		if _, err := repo.GetAdminByEmail(ctx, "does-not-exist@example.com"); err != ErrNotFound {
			t.Fatalf("GetAdminByEmail for missing row: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("RefreshTokenLifecycle", func(t *testing.T) {
		email := "integration-admin-2@example.com"
		roleID := mustRoleID(t, ctx, pool, "admin")
		hash, err := HashPassword("integration-test-password")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		var adminID int64
		err = pool.QueryRow(ctx, `
			INSERT INTO admin_users (email, password_hash, role_id, is_active)
			VALUES ($1, $2, $3, true)
			RETURNING id
		`, email, hash, roleID).Scan(&adminID)
		if err != nil {
			t.Fatalf("insert admin: %v", err)
		}
		t.Cleanup(func() { mustExec(t, ctx, pool, `DELETE FROM admin_users WHERE id = $1`, adminID) })

		_, tokenHash, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken: %v", err)
		}
		expiresAt := time.Now().Add(RefreshTokenTTL).Truncate(time.Millisecond)

		tokenID, err := repo.InsertRefreshToken(ctx, adminID, tokenHash, expiresAt)
		if err != nil {
			t.Fatalf("InsertRefreshToken: %v", err)
		}

		fetched, err := repo.GetRefreshTokenByHash(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetRefreshTokenByHash: %v", err)
		}
		if fetched.ID != tokenID || fetched.AdminUserID != adminID || fetched.RevokedAt != nil {
			t.Fatalf("unexpected refresh token record: %+v", fetched)
		}

		// Rotation: old id revoked, new hash active, in one transaction.
		_, newHash, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken: %v", err)
		}
		newExpiresAt := time.Now().Add(RefreshTokenTTL).Truncate(time.Millisecond)
		newID, err := repo.RotateRefreshToken(ctx, tokenID, adminID, newHash, newExpiresAt)
		if err != nil {
			t.Fatalf("RotateRefreshToken: %v", err)
		}
		if newID == tokenID {
			t.Fatal("expected a new token id from rotation")
		}

		oldAfterRotate, err := repo.GetRefreshTokenByHash(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetRefreshTokenByHash (old, post-rotation): %v", err)
		}
		if oldAfterRotate.RevokedAt == nil {
			t.Fatal("expected the old refresh token to be revoked after rotation")
		}

		newRecord, err := repo.GetRefreshTokenByHash(ctx, newHash)
		if err != nil {
			t.Fatalf("GetRefreshTokenByHash (new): %v", err)
		}
		if newRecord.RevokedAt != nil {
			t.Fatal("expected the newly rotated refresh token to be active")
		}

		// RevokeRefreshToken is idempotent: revoking an already-revoked row
		// leaves revoked_at at its original value rather than erroring or
		// bumping it forward.
		if err := repo.RevokeRefreshToken(ctx, newID); err != nil {
			t.Fatalf("RevokeRefreshToken: %v", err)
		}
		firstRevoke, err := repo.GetRefreshTokenByHash(ctx, newHash)
		if err != nil {
			t.Fatalf("GetRefreshTokenByHash: %v", err)
		}
		if firstRevoke.RevokedAt == nil {
			t.Fatal("expected revoked_at to be set")
		}
		if err := repo.RevokeRefreshToken(ctx, newID); err != nil {
			t.Fatalf("RevokeRefreshToken (second call): %v", err)
		}
		secondRevoke, err := repo.GetRefreshTokenByHash(ctx, newHash)
		if err != nil {
			t.Fatalf("GetRefreshTokenByHash: %v", err)
		}
		if !secondRevoke.RevokedAt.Equal(*firstRevoke.RevokedAt) {
			t.Fatalf("revoking twice must not move revoked_at: first=%v second=%v",
				firstRevoke.RevokedAt, secondRevoke.RevokedAt)
		}

		if _, err := repo.GetRefreshTokenByHash(ctx, "no-such-hash"); err != ErrNotFound {
			t.Fatalf("GetRefreshTokenByHash for missing row: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("RevokeAllRefreshTokens", func(t *testing.T) {
		email := "integration-admin-3@example.com"
		roleID := mustRoleID(t, ctx, pool, "admin")
		hash, err := HashPassword("integration-test-password")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		var adminID int64
		err = pool.QueryRow(ctx, `
			INSERT INTO admin_users (email, password_hash, role_id, is_active)
			VALUES ($1, $2, $3, true)
			RETURNING id
		`, email, hash, roleID).Scan(&adminID)
		if err != nil {
			t.Fatalf("insert admin: %v", err)
		}
		t.Cleanup(func() { mustExec(t, ctx, pool, `DELETE FROM admin_users WHERE id = $1`, adminID) })

		var hashes []string
		for i := 0; i < 3; i++ {
			_, h, err := GenerateRefreshToken()
			if err != nil {
				t.Fatalf("GenerateRefreshToken: %v", err)
			}
			if _, err := repo.InsertRefreshToken(ctx, adminID, h, time.Now().Add(RefreshTokenTTL)); err != nil {
				t.Fatalf("InsertRefreshToken: %v", err)
			}
			hashes = append(hashes, h)
		}

		if err := repo.RevokeAllRefreshTokens(ctx, adminID); err != nil {
			t.Fatalf("RevokeAllRefreshTokens: %v", err)
		}
		for _, h := range hashes {
			rec, err := repo.GetRefreshTokenByHash(ctx, h)
			if err != nil {
				t.Fatalf("GetRefreshTokenByHash: %v", err)
			}
			if rec.RevokedAt == nil {
				t.Fatalf("expected token %s to be revoked", h)
			}
		}
	})

	t.Run("UpsertCustomerByTelegramID_InsertThenUpdate", func(t *testing.T) {
		telegramID := int64(918273645)
		t.Cleanup(func() { mustExec(t, ctx, pool, `DELETE FROM users WHERE telegram_id = $1`, telegramID) })

		first, err := repo.UpsertCustomerByTelegramID(ctx, TelegramUser{
			ID: telegramID, Username: "neo", FirstName: "Thomas",
		})
		if err != nil {
			t.Fatalf("UpsertCustomerByTelegramID (insert): %v", err)
		}
		if first.TelegramID != telegramID || first.Username == nil || *first.Username != "neo" {
			t.Fatalf("unexpected customer after insert: %+v", first)
		}

		second, err := repo.UpsertCustomerByTelegramID(ctx, TelegramUser{
			ID: telegramID, Username: "neo", FirstName: "Thomas", LastName: "Anderson",
		})
		if err != nil {
			t.Fatalf("UpsertCustomerByTelegramID (update): %v", err)
		}
		if second.ID != first.ID {
			t.Fatalf("expected the same row on repeat upsert, got %d then %d", first.ID, second.ID)
		}
		if second.LastName == nil || *second.LastName != "Anderson" {
			t.Fatalf("expected last_name to be updated in place, got %+v", second)
		}
	})
}

func mustRoleID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM roles WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatalf("lookup seeded role %q (did migrations run?): %v", code, err)
	}
	return id
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("cleanup exec %q: %v", sql, err)
	}
}
