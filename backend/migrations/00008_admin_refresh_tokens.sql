-- +goose Up
-- Server-side session for staff (admin) auth, per docs/architecture.md
-- "Модель данных": logout must genuinely revoke access, so the refresh token
-- is stored here rather than being purely stateless. Redis was rejected for
-- this (Redis is cache/cart only, see Constraints); the storefront's initData
-- auth remains stateless and has no row here at all.
--
-- Only the token *hash* is stored, never the raw token, matching how
-- admin_users stores password_hash rather than the password itself.
CREATE TABLE admin_refresh_tokens (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    admin_user_id BIGINT      NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    token_hash    TEXT        NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    -- NULL = active. Revoked (logout, rotation, or account-wide revocation),
    -- never deleted, so a replayed old token can be told apart from one that
    -- simply never existed.
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Session lookup on refresh/logout: "find this hash" is a point lookup: the
-- app then checks revoked_at IS NULL AND expires_at > now() on the row.
CREATE UNIQUE INDEX admin_refresh_tokens_token_hash_key ON admin_refresh_tokens (token_hash);
-- FK column: Postgres does not auto-index it (unlike the PK side). Also
-- serves "revoke all sessions for this admin_user" on password change.
CREATE INDEX admin_refresh_tokens_admin_user_id_idx ON admin_refresh_tokens (admin_user_id);
-- Periodic cleanup of expired rows (worker/cron): "DELETE ... WHERE expires_at < now()".
CREATE INDEX admin_refresh_tokens_expires_at_idx ON admin_refresh_tokens (expires_at);

-- +goose Down
DROP TABLE admin_refresh_tokens;
