-- +goose Up
-- Inventory owns stock. Product information and stock levels change at
-- different rates and are edited by different roles, so they get different
-- tables and only inventory writes this one.
--
-- available_stock is stored rather than derived: every inventory operation then
-- becomes a single guarded column update, and deduct stops having to touch two
-- columns atomically. Total on hand is derived as available_stock + reserved_stock.
--
-- Keyed by product_id alone -- no warehouse column. Multi-warehouse is not a
-- column, it is an allocation strategy plus per-line reservation records plus
-- split shipments; see ARCHITECTURE-LIMITATIONS.md for what extending this costs.
CREATE TABLE IF NOT EXISTS inventory_levels (
    product_id      UUID PRIMARY KEY REFERENCES products(id),
    available_stock INT NOT NULL DEFAULT 0,
    reserved_stock  INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT inventory_available_check CHECK (available_stock >= 0),
    CONSTRAINT inventory_reserved_check  CHECK (reserved_stock  >= 0)
);

CREATE TRIGGER update_inventory_levels_updated_at BEFORE UPDATE ON inventory_levels
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Backfill every product, including soft-deleted ones: a soft-deleted product
-- may still hold reserved stock for an in-flight order, and dropping that row
-- would strand the reservation.
INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
SELECT id, stock_quantity - reserved_quantity, reserved_quantity
FROM products
ON CONFLICT (product_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS inventory_levels;
