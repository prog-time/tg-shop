-- +goose Up
-- RBAC and user identity. Per docs/architecture.md the storefront and admin
-- panel use different auth mechanisms: `users` are Telegram customers
-- (authenticated via initData), `admin_users` are staff with a password login
-- and a role (admin / order_manager / content_manager, see docs/database.md).
CREATE TABLE roles (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL,
    code        TEXT        NOT NULL,
    permissions JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE UNIQUE INDEX roles_code_key ON roles (code);

CREATE TABLE admin_users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role_id       BIGINT      NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX admin_users_email_key ON admin_users (email);
-- FK column: Postgres does not auto-index it (unlike the PK side).
CREATE INDEX admin_users_role_id_idx ON admin_users (role_id);

CREATE TABLE users (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT      NOT NULL,
    username    TEXT,
    first_name  TEXT,
    last_name   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_telegram_id_key ON users (telegram_id);

-- +goose Down
DROP TABLE users;
DROP TABLE admin_users;
DROP TABLE roles;
