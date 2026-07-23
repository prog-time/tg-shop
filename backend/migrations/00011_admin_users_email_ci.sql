-- +goose Up
-- Fixes a silent lockout: 00002 made `admin_users.email` unique on the raw
-- string, while the login path normalizes the submitted email to lowercase
-- (internal/auth Service.Login). An account seeded as `Admin@shop.io` could
-- therefore never authenticate — the lookup for `admin@shop.io` simply missed,
-- with no error anywhere. The same gap also let two accounts differing only in
-- case coexist, each claiming to be "the" admin for that address.
--
-- Email is case-insensitive for routing purposes in every mailbox provider
-- that matters here, so the constraint follows the login path rather than the
-- other way round: uniqueness on lower(email), which the equality lookup in
-- internal/auth Repo.GetAdminByEmail also uses as its index.
--
-- Existing rows are folded to lowercase first. That is safe today because no
-- module creates admin_users yet (admin CRUD is a later slice); if duplicates
-- differing only in case somehow exist, this migration fails loudly on the
-- unique index rather than silently dropping an account.
UPDATE admin_users SET email = lower(email) WHERE email <> lower(email);

DROP INDEX admin_users_email_key;
CREATE UNIQUE INDEX admin_users_email_key ON admin_users (lower(email));

-- +goose Down
DROP INDEX admin_users_email_key;
CREATE UNIQUE INDEX admin_users_email_key ON admin_users (email);
