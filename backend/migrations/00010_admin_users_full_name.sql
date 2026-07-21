-- +goose Up
-- Additive column backing `AdminUser.full_name` in docs/api/openapi.yaml
-- (components/schemas/AdminUser). The contract's schema comment flags this
-- explicitly as a judgment call beyond the indicative docs/database.md ERD
-- ("confirm and add the column during implementation, or drop this field").
-- Implementation confirms it: nullable, display-only, never used for
-- authentication or RBAC — safe to add without a two-deployment
-- expand/contract because it has no NOT NULL/default to backfill.
ALTER TABLE admin_users ADD COLUMN full_name TEXT;

-- +goose Down
ALTER TABLE admin_users DROP COLUMN full_name;
