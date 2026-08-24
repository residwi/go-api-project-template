-- +goose Up
CREATE TABLE IF NOT EXISTS job_queue (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue         VARCHAR(50) NOT NULL,
    kind          VARCHAR(100) NOT NULL,
    payload       JSONB NOT NULL,
    dedup_key     TEXT,
    group_key     TEXT,
    status        VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 3,
    last_error    TEXT,
    locked_until  TIMESTAMPTZ,
    run_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT job_queue_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'dead', 'cancelled'))
);

CREATE UNIQUE INDEX ux_job_queue_active ON job_queue(dedup_key)
    WHERE dedup_key IS NOT NULL AND status IN ('pending', 'processing');
CREATE INDEX idx_job_queue_ready ON job_queue(queue, run_at) WHERE status = 'pending';
CREATE INDEX idx_job_queue_group ON job_queue(group_key) WHERE group_key IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS job_queue;
