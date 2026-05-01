-- +goose Up
CREATE TABLE IF NOT EXISTS shipments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders(id),
    carrier         VARCHAR(100),
    tracking_number VARCHAR(255),
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    shipped_at      TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT shipments_status_check CHECK (status IN (
        'pending', 'shipped', 'in_transit', 'delivered', 'returned'
    ))
);

CREATE UNIQUE INDEX ux_shipments_order ON shipments(order_id);
CREATE INDEX idx_shipments_tracking ON shipments(tracking_number);

CREATE TRIGGER update_shipments_updated_at BEFORE UPDATE ON shipments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS shipments;
