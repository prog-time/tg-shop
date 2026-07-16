-- +goose Up
-- Media files (MinIO objects), products and the facet link table that powers
-- storefront filtering. `catalog` (docs/architecture.md) is the only writer of
-- these tables and rebuilds product_facets on every product write.
CREATE TABLE media (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bucket       TEXT        NOT NULL,
    object_key   TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    size         BIGINT      NOT NULL,
    width        INT,
    height       INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX media_object_key_key ON media (object_key);

-- Single-currency shop (no currency column, per docs/architecture.md).
CREATE TABLE products (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id BIGINT      NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    name       TEXT         NOT NULL,
    slug       TEXT         NOT NULL,
    price      NUMERIC(12, 2) NOT NULL CHECK (price >= 0),
    stock      INT          NOT NULL DEFAULT 0 CHECK (stock >= 0),
    -- Dictionary value ids, not text (docs/architecture.md); validated in Go
    -- against field_definitions before write.
    attributes JSONB        NOT NULL DEFAULT '{}'::jsonb,
    is_active  BOOLEAN      NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX products_slug_key ON products (slug);
CREATE INDEX products_category_id_idx ON products (category_id);
-- Attribute lookups that need filtering/sorting are not done via this GIN
-- index (that would violate "filter fields are a column/product_facets row,
-- not JSONB search" — see docs/architecture.md); it only speeds up ad-hoc
-- containment queries against the display-only parts of attributes.
CREATE INDEX products_attributes_gin_idx ON products USING GIN (attributes);

CREATE TABLE product_images (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    media_id   BIGINT NOT NULL REFERENCES media (id) ON DELETE RESTRICT,
    position   INT    NOT NULL DEFAULT 0,
    alt        TEXT   NOT NULL DEFAULT ''
);

CREATE INDEX product_images_product_id_idx ON product_images (product_id);
CREATE INDEX product_images_media_id_idx ON product_images (media_id);

-- Denormalized "product <-> dictionary value" link, rebuilt by catalog on
-- every product write. Storefront facet filters query this table with plain
-- B-tree indexes instead of scanning products.attributes JSONB.
CREATE TABLE product_facets (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id          BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    field_definition_id BIGINT NOT NULL REFERENCES field_definitions (id) ON DELETE CASCADE,
    dictionary_value_id BIGINT NOT NULL REFERENCES dictionary_values (id) ON DELETE CASCADE
);

-- Also serves as the "all facets for a product" lookup (rebuild/delete) since
-- product_id is the leading column.
CREATE UNIQUE INDEX product_facets_product_field_value_key
    ON product_facets (product_id, field_definition_id, dictionary_value_id);
-- Storefront filter query: "products having value V for field F".
CREATE INDEX product_facets_field_value_idx ON product_facets (field_definition_id, dictionary_value_id);
CREATE INDEX product_facets_dictionary_value_id_idx ON product_facets (dictionary_value_id);

-- +goose Down
DROP TABLE product_facets;
DROP TABLE product_images;
DROP TABLE products;
DROP TABLE media;
