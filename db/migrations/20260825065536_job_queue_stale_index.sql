-- +goose Up
CREATE INDEX idx_job_queue_stale ON job_queue(locked_until) WHERE status = 'processing';

-- +goose Down
DROP INDEX IF EXISTS idx_job_queue_stale;
