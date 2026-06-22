-- +goose Up
-- Persist whether an order's stock is currently deducted (sold) and whether its
-- inventory hold has already been reversed. The status alone cannot answer this:
-- fulfillment_failed is reachable from both reserved-only (pre-paid) and deducted
-- (paid) states, so deriving release-vs-restock from status mis-reverses stock.
-- These flags are set atomically with the status transition that changes them.
ALTER TABLE orders
    ADD COLUMN stock_deducted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN stock_reversed BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE orders
    DROP COLUMN stock_deducted,
    DROP COLUMN stock_reversed;
