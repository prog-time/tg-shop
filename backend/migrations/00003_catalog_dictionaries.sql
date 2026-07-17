-- +goose Up
-- Flexible catalog core (ADR-002): dictionaries + values feed field_definitions,
-- which describe per-category product attributes. Values and field definitions
-- are never hard-deleted once used — they are archived / deprecated instead,
-- so a rename or removal never rewrites historical products or orders.
CREATE TABLE dictionaries (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    code       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX dictionaries_code_key ON dictionaries (code);

CREATE TABLE dictionary_values (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dictionary_id BIGINT      NOT NULL REFERENCES dictionaries (id) ON DELETE CASCADE,
    value         TEXT        NOT NULL,
    position      INT         NOT NULL DEFAULT 0,
    -- NULL = active. A value in use by a product is archived, never deleted:
    -- archived values keep rendering on existing products but are not offered
    -- for new ones (docs/architecture.md, "Правила модели").
    archived_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Needed for listing all values of a dictionary including archived ones (the
-- unique index below only covers active rows, so it can't serve that lookup).
CREATE INDEX dictionary_values_dictionary_id_idx ON dictionary_values (dictionary_id);
-- A dictionary should not contain the same *active* value twice. Partial on
-- archived_at IS NULL: an archived value's text must not squat the slot
-- forever, or a manager could never re-add an active value with that text
-- (docs/architecture.md: values are archived, not deleted).
CREATE UNIQUE INDEX dictionary_values_dictionary_id_value_key
    ON dictionary_values (dictionary_id, value) WHERE archived_at IS NULL;
-- Lists values of a dictionary excluding archived ones, in display order.
CREATE INDEX dictionary_values_active_idx ON dictionary_values (dictionary_id, position) WHERE archived_at IS NULL;

-- Self-referencing category tree.
CREATE TABLE categories (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id  BIGINT      REFERENCES categories (id) ON DELETE SET NULL,
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL,
    position   INT         NOT NULL DEFAULT 0,
    is_active  BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX categories_slug_key ON categories (slug);
CREATE INDEX categories_parent_id_idx ON categories (parent_id);

-- Registry of per-category attribute fields. A field is never deleted, only
-- marked deprecated; its type can never change once created (a new field is
-- created instead and data migrated) — see docs/architecture.md.
CREATE TABLE field_definitions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id   BIGINT      NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    -- Only set (and only meaningful) for type IN ('dictionary', 'dictionary_multi').
    dictionary_id BIGINT      REFERENCES dictionaries (id) ON DELETE RESTRICT,
    code          TEXT        NOT NULL,
    label         TEXT        NOT NULL,
    type          TEXT        NOT NULL,
    required      BOOLEAN     NOT NULL DEFAULT false,
    validation    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    position      INT         NOT NULL DEFAULT 0,
    show_in_list  BOOLEAN     NOT NULL DEFAULT false,
    is_deprecated BOOLEAN     NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Leading column of this unique index also serves plain category_id lookups,
-- so no separate field_definitions_category_id_idx is needed.
CREATE UNIQUE INDEX field_definitions_category_id_code_key ON field_definitions (category_id, code);
CREATE INDEX field_definitions_dictionary_id_idx ON field_definitions (dictionary_id);

-- +goose Down
DROP TABLE field_definitions;
DROP TABLE categories;
DROP TABLE dictionary_values;
DROP TABLE dictionaries;
