package auth

import "context"

type ctxKey int

const (
	telegramUserKey ctxKey = iota
	adminIdentityKey
)

// TelegramUser is the identity extracted from a verified Telegram Web App
// `initData` payload (see initdata.go). It is never trusted unless it came
// out of VerifyInitData.
type TelegramUser struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
}

// AdminIdentity is the staff identity resolved by RequireAdminJWT: the access
// JWT is only proof of *who*; role and active status are always re-read from
// PostgreSQL (admin_users/roles) on every request so a deactivation or role
// change takes effect immediately, without waiting for the 15-minute access
// token to expire.
type AdminIdentity struct {
	ID       int64
	Email    string
	Role     string
	FullName *string
}

// WithTelegramUser attaches u to ctx. Called by RequireInitData once
// verification succeeds.
func WithTelegramUser(ctx context.Context, u TelegramUser) context.Context {
	return context.WithValue(ctx, telegramUserKey, u)
}

// TelegramUserFromContext returns the Telegram user attached by
// RequireInitData, or ok=false if none is present (e.g. the route isn't
// behind that middleware).
func TelegramUserFromContext(ctx context.Context) (TelegramUser, bool) {
	u, ok := ctx.Value(telegramUserKey).(TelegramUser)
	return u, ok
}

// WithAdminIdentity attaches a to ctx. Called by RequireAdminJWT once the
// token is verified and the admin is loaded and confirmed active.
func WithAdminIdentity(ctx context.Context, a AdminIdentity) context.Context {
	return context.WithValue(ctx, adminIdentityKey, a)
}

// AdminIdentityFromContext returns the staff identity attached by
// RequireAdminJWT, or ok=false if none is present.
func AdminIdentityFromContext(ctx context.Context) (AdminIdentity, bool) {
	a, ok := ctx.Value(adminIdentityKey).(AdminIdentity)
	return a, ok
}
