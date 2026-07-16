-- +goose Up
-- Delivery and payment methods managed by admins (adminmod). Orders reference
-- the live row for reporting, but freeze name/price as a snapshot at checkout
-- time (see 00007_orders.sql) so editing/retiring a method never rewrites
-- past orders.
CREATE TABLE delivery_methods (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name      TEXT           NOT NULL,
    code      TEXT           NOT NULL,
    price     NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (price >= 0),
    config    JSONB          NOT NULL DEFAULT '{}'::jsonb,
    is_active BOOLEAN        NOT NULL DEFAULT true
);

CREATE UNIQUE INDEX delivery_methods_code_key ON delivery_methods (code);

CREATE TABLE payment_methods (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name      TEXT    NOT NULL,
    code      TEXT    NOT NULL,
    provider  TEXT    NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE UNIQUE INDEX payment_methods_code_key ON payment_methods (code);

-- +goose Down
DROP TABLE payment_methods;
DROP TABLE delivery_methods;
