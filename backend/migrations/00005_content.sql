-- +goose Up
-- Blog/news content, managed by staff via adminmod, read by the storefront.
CREATE TABLE posts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title          TEXT        NOT NULL,
    slug           TEXT        NOT NULL,
    body           TEXT        NOT NULL,
    cover_media_id BIGINT      REFERENCES media (id) ON DELETE SET NULL,
    -- NULL = draft, not shown on the storefront.
    published_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX posts_slug_key ON posts (slug);
CREATE INDEX posts_cover_media_id_idx ON posts (cover_media_id);
-- Storefront listing: published posts ordered newest first; drafts excluded.
CREATE INDEX posts_published_at_idx ON posts (published_at DESC) WHERE published_at IS NOT NULL;

-- +goose Down
DROP TABLE posts;
