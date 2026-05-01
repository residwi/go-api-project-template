-- +goose Up
CREATE TABLE IF NOT EXISTS products (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(255) NOT NULL,
    slug              VARCHAR(255) NOT NULL UNIQUE,
    description       TEXT,
    price             BIGINT NOT NULL,
    compare_at_price  BIGINT,
    currency          VARCHAR(3) NOT NULL DEFAULT 'USD',
    sku               VARCHAR(100) UNIQUE,
    category_id       UUID REFERENCES categories(id),
    status            VARCHAR(50) NOT NULL DEFAULT 'draft',
    stock_quantity    INT NOT NULL DEFAULT 0,
    reserved_quantity INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,

    CONSTRAINT products_status_check CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT products_price_check CHECK (price >= 0),
    CONSTRAINT products_stock_check CHECK (stock_quantity >= 0),
    CONSTRAINT products_reserved_check CHECK (reserved_quantity >= 0 AND reserved_quantity <= stock_quantity)
);

CREATE INDEX idx_products_slug ON products(slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_category ON products(category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_status ON products(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_feed ON products(created_at DESC, id DESC) WHERE status = 'published' AND deleted_at IS NULL;

CREATE TRIGGER update_products_updated_at BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS product_images (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    alt_text   VARCHAR(255),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_product_images_product ON product_images(product_id);

-- +goose Down
DROP TABLE IF EXISTS product_images;
DROP TABLE IF EXISTS products;
