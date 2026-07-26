-- +goose Up
-- inventory_levels is now the only writer and reader of stock. These columns
-- have had no reader since product stopped selecting them, so dropping them
-- makes the ownership rule enforceable: a grep for stock_quantity outside
-- inventory is now a compile error rather than a code review note.
ALTER TABLE products
    DROP CONSTRAINT products_stock_check,
    DROP CONSTRAINT products_reserved_check,
    DROP COLUMN stock_quantity,
    DROP COLUMN reserved_quantity;

-- +goose Down
ALTER TABLE products
    ADD COLUMN stock_quantity    INT NOT NULL DEFAULT 0,
    ADD COLUMN reserved_quantity INT NOT NULL DEFAULT 0;

UPDATE products p
SET stock_quantity    = i.available_stock + i.reserved_stock,
    reserved_quantity = i.reserved_stock
FROM inventory_levels i
WHERE i.product_id = p.id;

ALTER TABLE products
    ADD CONSTRAINT products_stock_check CHECK (stock_quantity >= 0),
    ADD CONSTRAINT products_reserved_check CHECK (reserved_quantity >= 0 AND reserved_quantity <= stock_quantity);
