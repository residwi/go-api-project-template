-- +goose Up
-- Trigram indexes so the product search (ILIKE '%term%') is index-backed instead
-- of forcing a sequential scan over the products table on every search request.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_products_name_trgm ON products USING gin (name gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_description_trgm ON products USING gin (description gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_sku_trgm ON products USING gin (sku gin_trgm_ops) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_products_sku_trgm;
DROP INDEX IF EXISTS idx_products_description_trgm;
DROP INDEX IF EXISTS idx_products_name_trgm;
DROP EXTENSION IF EXISTS pg_trgm;
