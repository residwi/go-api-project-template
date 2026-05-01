-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       VARCHAR(50) NOT NULL,
    title      VARCHAR(255) NOT NULL,
    body       TEXT,
    is_read    BOOLEAN NOT NULL DEFAULT false,
    data       JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_feed ON notifications(user_id, created_at DESC, id DESC);
CREATE INDEX idx_notifications_unread ON notifications(user_id) WHERE is_read = false;

CREATE TABLE IF NOT EXISTS notification_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         VARCHAR(50) NOT NULL,
    title        VARCHAR(255) NOT NULL,
    body         TEXT,
    data         JSONB,
    status       VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT notification_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX idx_notification_jobs_pending ON notification_jobs(created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS notification_jobs;
DROP TABLE IF EXISTS notifications;
