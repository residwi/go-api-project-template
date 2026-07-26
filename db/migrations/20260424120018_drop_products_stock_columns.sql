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

-- An explicit LEFT JOIN over every product, not `FROM inventory_levels` (which
-- only touches rows with a matching level and would otherwise leave a
-- levelless product's reconstruction to whatever the ADD COLUMN default
-- happened to be -- an implementation detail, not a decision). A product with
-- no inventory_levels row (e.g. Create committed but a later EnsureLevel
-- never ran, see product.Service.Create) reconstructs as 0/0: there is no
-- other record of its stock from the modular period, so zero is the only
-- defensible value, and COALESCE makes that choice explicit rather than
-- incidental.
UPDATE products p
SET stock_quantity    = COALESCE(i.available_stock, 0) + COALESCE(i.reserved_stock, 0),
    reserved_quantity = COALESCE(i.reserved_stock, 0)
FROM products p2
LEFT JOIN inventory_levels i ON i.product_id = p2.id
WHERE p.id = p2.id;

ALTER TABLE products
    ADD CONSTRAINT products_stock_check CHECK (stock_quantity >= 0),
    ADD CONSTRAINT products_reserved_check CHECK (reserved_quantity >= 0 AND reserved_quantity <= stock_quantity);
