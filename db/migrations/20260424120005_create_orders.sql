-- +goose Up
CREATE TABLE IF NOT EXISTS orders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    idempotency_key  VARCHAR(255),
    request_hash     VARCHAR(64),
    status           VARCHAR(50) NOT NULL DEFAULT 'awaiting_payment',
    subtotal_amount  BIGINT NOT NULL,
    discount_amount  BIGINT NOT NULL DEFAULT 0,
    total_amount     BIGINT NOT NULL,
    coupon_code      VARCHAR(50),
    currency         VARCHAR(3) NOT NULL DEFAULT 'USD',
    shipping_address JSONB,
    billing_address  JSONB,
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT orders_status_check CHECK (status IN (
        'awaiting_payment', 'payment_processing',
        'paid', 'processing', 'shipped', 'delivered',
        'cancelled', 'expired', 'refunded', 'fulfillment_failed'
    )),
    CONSTRAINT orders_subtotal_amount_check CHECK (subtotal_amount >= 0),
    CONSTRAINT orders_total_amount_check CHECK (total_amount >= 0),
    CONSTRAINT orders_discount_amount_check CHECK (discount_amount >= 0 AND discount_amount <= subtotal_amount)
);

CREATE UNIQUE INDEX ux_orders_idempotency ON orders(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_orders_user_feed ON orders(user_id, created_at DESC, id DESC);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_status_feed ON orders(status, created_at DESC, id DESC);
CREATE INDEX idx_orders_awaiting_updated ON orders(updated_at) WHERE status = 'awaiting_payment';
CREATE INDEX idx_orders_processing_updated ON orders(updated_at) WHERE status = 'payment_processing';

CREATE TABLE IF NOT EXISTS order_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES products(id),
    product_name VARCHAR(255) NOT NULL,
    price        BIGINT NOT NULL,
    quantity     INT NOT NULL,
    subtotal     BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT order_items_quantity_check CHECK (quantity > 0),
    CONSTRAINT order_items_subtotal_check CHECK (subtotal >= 0)
);

CREATE INDEX idx_order_items_order ON order_items(order_id);

CREATE TRIGGER update_orders_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
