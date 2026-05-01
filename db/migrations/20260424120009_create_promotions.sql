-- +goose Up
CREATE TABLE IF NOT EXISTS promotions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code             VARCHAR(50) NOT NULL UNIQUE,
    type             VARCHAR(50) NOT NULL,
    value            BIGINT NOT NULL,
    min_order_amount BIGINT DEFAULT 0,
    max_discount     BIGINT,
    max_uses         INT,
    used_count       INT NOT NULL DEFAULT 0,
    starts_at        TIMESTAMPTZ NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT promotions_type_check CHECK (type IN ('percentage', 'fixed_amount')),
    CONSTRAINT promotions_value_check CHECK (value > 0),
    CONSTRAINT promotions_used_count_check CHECK (used_count >= 0)
);

CREATE INDEX idx_promotions_code ON promotions(code) WHERE active = true;

CREATE TABLE IF NOT EXISTS coupon_usages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id  UUID NOT NULL REFERENCES promotions(id),
    user_id    UUID NOT NULL REFERENCES users(id),
    order_id   UUID NOT NULL REFERENCES orders(id),
    discount   BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT coupon_usage_unique UNIQUE (coupon_id, user_id, order_id),
    CONSTRAINT coupon_usage_per_user UNIQUE (coupon_id, user_id)
);

CREATE TRIGGER update_promotions_updated_at BEFORE UPDATE ON promotions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS coupon_usages;
DROP TABLE IF EXISTS promotions;
