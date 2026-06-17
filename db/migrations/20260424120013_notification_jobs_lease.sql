-- +goose Up
-- Add a lease so a notification job whose worker dies mid-processing can be
-- reclaimed instead of being stuck in 'processing' forever (mirrors payment_jobs).
ALTER TABLE notification_jobs ADD COLUMN locked_until TIMESTAMPTZ;

CREATE INDEX idx_notification_jobs_reclaim ON notification_jobs(locked_until) WHERE status = 'processing';

-- +goose Down
DROP INDEX IF EXISTS idx_notification_jobs_reclaim;
ALTER TABLE notification_jobs DROP COLUMN IF EXISTS locked_until;
