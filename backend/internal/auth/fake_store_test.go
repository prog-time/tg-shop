package auth

import (
	"context"
	"sync"
	"time"
)

// fakeStore is an in-memory Store used by service_test.go and
// handlers_test.go so business-logic and HTTP-layer coverage doesn't require
// a live Postgres connection. It mirrors the semantics of Repo's actual SQL
// (see repo.go and repo_integration_test.go, which exercises that SQL for
// real) closely enough for those tests: unique constraints, revoke-is-a-
// no-op-if-already-revoked, and upsert-by-telegram_id.
type fakeStore struct {
	mu sync.Mutex

	adminsByID    map[int64]*AdminRecord
	adminsByEmail map[string]*AdminRecord

	refreshTokens map[string]*RefreshTokenRecord // keyed by token_hash
	nextTokenID   int64

	customersByTelegramID map[int64]*CustomerRecord
	nextCustomerID        int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		adminsByID:            map[int64]*AdminRecord{},
		adminsByEmail:         map[string]*AdminRecord{},
		refreshTokens:         map[string]*RefreshTokenRecord{},
		customersByTelegramID: map[int64]*CustomerRecord{},
	}
}

func (f *fakeStore) addAdmin(a *AdminRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adminsByID[a.ID] = a
	f.adminsByEmail[a.Email] = a
}

func (f *fakeStore) GetAdminByEmail(_ context.Context, email string) (*AdminRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.adminsByEmail[email]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (f *fakeStore) GetAdminByID(_ context.Context, id int64) (*AdminRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.adminsByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (f *fakeStore) InsertRefreshToken(_ context.Context, adminUserID int64, tokenHash string, expiresAt time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTokenID++
	id := f.nextTokenID
	f.refreshTokens[tokenHash] = &RefreshTokenRecord{
		ID:          id,
		AdminUserID: adminUserID,
		ExpiresAt:   expiresAt,
	}
	return id, nil
}

func (f *fakeStore) GetRefreshTokenByHash(_ context.Context, tokenHash string) (*RefreshTokenRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.refreshTokens[tokenHash]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeStore) RevokeRefreshToken(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.refreshTokens {
		if t.ID == id && t.RevokedAt == nil {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeStore) RevokeAllRefreshTokens(_ context.Context, adminUserID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.refreshTokens {
		if t.AdminUserID == adminUserID && t.RevokedAt == nil {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeStore) RotateRefreshToken(_ context.Context, oldID, adminUserID int64, newTokenHash string, newExpiresAt time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.refreshTokens {
		if t.ID == oldID && t.RevokedAt == nil {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	f.nextTokenID++
	id := f.nextTokenID
	f.refreshTokens[newTokenHash] = &RefreshTokenRecord{
		ID:          id,
		AdminUserID: adminUserID,
		ExpiresAt:   newExpiresAt,
	}
	return id, nil
}

func (f *fakeStore) UpsertCustomerByTelegramID(_ context.Context, u TelegramUser) (*CustomerRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

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

	if existing, ok := f.customersByTelegramID[u.ID]; ok {
		existing.Username = username
		existing.FirstName = firstName
		existing.LastName = lastName
		cp := *existing
		return &cp, nil
	}

	f.nextCustomerID++
	c := &CustomerRecord{
		ID:         f.nextCustomerID,
		TelegramID: u.ID,
		Username:   username,
		FirstName:  firstName,
		LastName:   lastName,
		CreatedAt:  time.Now(),
	}
	f.customersByTelegramID[u.ID] = c
	cp := *c
	return &cp, nil
}

var _ Store = (*fakeStore)(nil)
