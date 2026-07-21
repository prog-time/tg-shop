-- +goose Up
-- RBAC roles, per docs/architecture.md / docs/database.md: `roles.code` is a
-- fixed enum (`admin`, `order_manager`, `content_manager`), not a
-- manager-editable reference table like `dictionaries` (ADR-002 flexibility
-- applies to catalog attributes only). Without these rows `admin_users` could
-- never be created (role_id is NOT NULL, RESTRICT on delete) and the
-- contract's `RoleName` enum (docs/api/openapi.yaml) would have nothing to
-- reference. Seeding is idempotent-by-migration (goose runs this exactly
-- once), not runtime upsert logic.
INSERT INTO roles (name, code, permissions) VALUES
    ('Administrator', 'admin', '{}'::jsonb),
    ('Order Manager', 'order_manager', '{}'::jsonb),
    ('Content Manager', 'content_manager', '{}'::jsonb);

-- +goose Down
DELETE FROM roles WHERE code IN ('admin', 'order_manager', 'content_manager');
