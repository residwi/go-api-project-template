-- +goose Up
CREATE TABLE IF NOT EXISTS reviews (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    product_id UUID NOT NULL REFERENCES products(id),
    order_id   UUID NOT NULL REFERENCES orders(id),
    rating     INT NOT NULL,
    title      VARCHAR(255),
    body       TEXT,
    status     VARCHAR(50) NOT NULL DEFAULT 'published',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT reviews_rating_check CHECK (rating >= 1 AND rating <= 5),
    CONSTRAINT reviews_one_per_product UNIQUE (user_id, product_id)
);

CREATE INDEX idx_reviews_product_feed ON reviews(product_id, created_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX idx_reviews_user ON reviews(user_id);

CREATE TRIGGER update_reviews_updated_at BEFORE UPDATE ON reviews
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS reviews;
