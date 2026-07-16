-- +goose Up
-- Orders, line items and payments. `orders` is written by the Orders Module
-- inside a single transaction (cart -> stock decrement with row lock ->
-- delivery calc -> order + outbox event), per docs/architecture.md. Line
-- items and the delivery/payment method freeze name+price as a snapshot so
-- catalog or method edits never rewrite order history.
CREATE TABLE orders (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id                 BIGINT         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    -- Live reference for reporting/joins; nullable because the method row can
    -- be retired later. The frozen name/price below is what history relies on.
    delivery_method_id      BIGINT         REFERENCES delivery_methods (id) ON DELETE SET NULL,
    payment_method_id       BIGINT         REFERENCES payment_methods (id) ON DELETE SET NULL,
    status                  TEXT           NOT NULL DEFAULT 'new',
    total                   NUMERIC(12, 2) NOT NULL CHECK (total >= 0),
    delivery_name_snapshot  TEXT           NOT NULL,
    delivery_price_snapshot NUMERIC(12, 2) NOT NULL CHECK (delivery_price_snapshot >= 0),
    payment_name_snapshot   TEXT           NOT NULL,
    delivery_address        JSONB          NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_id_idx ON orders (user_id);
CREATE INDEX orders_delivery_method_id_idx ON orders (delivery_method_id);
CREATE INDEX orders_payment_method_id_idx ON orders (payment_method_id);
-- Admin order list / status dashboards filter and sort by these.
CREATE INDEX orders_status_created_at_idx ON orders (status, created_at DESC);

CREATE TABLE order_items (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id            BIGINT         NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    -- Traceability only, nullable: the product may later be deleted or change;
    -- the snapshot columns below are the source of truth for order history.
    product_id          BIGINT         REFERENCES products (id) ON DELETE SET NULL,
    name_snapshot       TEXT           NOT NULL,
    price_snapshot      NUMERIC(12, 2) NOT NULL CHECK (price_snapshot >= 0),
    qty                 INT            NOT NULL CHECK (qty > 0),
    attributes_snapshot JSONB          NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX order_items_order_id_idx ON order_items (order_id);
CREATE INDEX order_items_product_id_idx ON order_items (product_id);

CREATE TABLE payments (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id            BIGINT         NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    provider            TEXT           NOT NULL,
    -- Provider's id for the payment; callbacks are matched and de-duplicated
    -- on this value so a replayed webhook cannot double-process a payment.
    provider_payment_id TEXT           NOT NULL,
    status              TEXT           NOT NULL DEFAULT 'pending',
    amount              NUMERIC(12, 2) NOT NULL CHECK (amount >= 0),
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE INDEX payments_order_id_idx ON payments (order_id);
CREATE UNIQUE INDEX payments_provider_payment_id_key ON payments (provider_payment_id);

-- +goose Down
DROP TABLE payments;
DROP TABLE order_items;
DROP TABLE orders;
