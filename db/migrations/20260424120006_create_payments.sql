-- +goose Up
CREATE TABLE IF NOT EXISTS payments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID NOT NULL REFERENCES orders(id),
    amount            BIGINT NOT NULL,
    currency          VARCHAR(3) NOT NULL DEFAULT 'USD',
    status            VARCHAR(50) NOT NULL DEFAULT 'pending',
    method            VARCHAR(50),
    payment_method_id VARCHAR(255),
    payment_url       TEXT,
    gateway_txn_id    VARCHAR(255),
    gateway_response  JSONB,
    paid_at           TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT payments_status_check CHECK (status IN (
        'pending', 'processing', 'success', 'failed', 'cancelled', 'requires_review', 'refunded'
    )),
    CONSTRAINT payments_amount_check CHECK (amount >= 0)
);

CREATE UNIQUE INDEX ux_payments_order_active ON payments(order_id) WHERE status IN ('pending', 'processing', 'requires_review');
CREATE INDEX idx_payments_order ON payments(order_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_gateway_txn ON payments(gateway_txn_id) WHERE gateway_txn_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS payment_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id    UUID NOT NULL REFERENCES payments(id),
    order_id      UUID NOT NULL REFERENCES orders(id),
    action        VARCHAR(50) NOT NULL DEFAULT 'charge',
    status        VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 3,
    last_error    TEXT,
    locked_until  TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    inventory_action VARCHAR(50),

    CONSTRAINT payment_jobs_action_check CHECK (action IN ('charge', 'refund')),
    CONSTRAINT payment_jobs_inventory_action_check CHECK (inventory_action IS NULL OR inventory_action IN ('release', 'restock')),
    CONSTRAINT payment_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled'))
);

CREATE UNIQUE INDEX ux_payment_jobs_active ON payment_jobs(payment_id, action) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_payment_jobs_pending ON payment_jobs(next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_payment_jobs_stale ON payment_jobs(locked_until) WHERE status = 'processing';

CREATE TRIGGER update_payments_updated_at BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_payment_jobs_updated_at BEFORE UPDATE ON payment_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS payment_jobs;
DROP TABLE IF EXISTS payments;
