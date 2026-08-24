-- +goose Up
ALTER TABLE notifications DROP COLUMN type;
ALTER TABLE notifications DROP COLUMN data;

DROP TABLE IF EXISTS payment_jobs;
DROP TABLE IF EXISTS notification_jobs;

-- +goose Down
CREATE TABLE IF NOT EXISTS notification_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    type         VARCHAR(50) NOT NULL,
    title        VARCHAR(255) NOT NULL,
    body         TEXT,
    data         JSONB,
    status       VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until TIMESTAMPTZ,

    CONSTRAINT notification_jobs_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX idx_notification_jobs_pending ON notification_jobs(created_at) WHERE status = 'pending';
CREATE INDEX idx_notification_jobs_reclaim ON notification_jobs(locked_until) WHERE status = 'processing';

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

    CONSTRAINT payment_jobs_action_check CHECK (action IN ('charge', 'refund')),
    CONSTRAINT payment_jobs_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled'))
);

CREATE UNIQUE INDEX ux_payment_jobs_active ON payment_jobs(payment_id, action)
    WHERE status IN ('pending', 'processing');
CREATE INDEX idx_payment_jobs_pending ON payment_jobs(next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_payment_jobs_stale ON payment_jobs(locked_until) WHERE status = 'processing';

CREATE TRIGGER update_payment_jobs_updated_at BEFORE UPDATE ON payment_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notifications ADD COLUMN data JSONB;
ALTER TABLE notifications ADD COLUMN type VARCHAR(50);
