-- +goose Up
-- The transactional outbox (ADR-004). Domain operations write an event row in
-- the same transaction as their data; the relay in worker publishes pending
-- rows to RabbitMQ after commit and marks them published. Types follow
-- docs/database.md.
CREATE TABLE outbox (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

-- Relay lookup: oldest unpublished rows first. The partial index keeps it small
-- as published rows accumulate; the age of the oldest pending row is alerted on.
CREATE INDEX outbox_pending_idx ON outbox (created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE outbox;
