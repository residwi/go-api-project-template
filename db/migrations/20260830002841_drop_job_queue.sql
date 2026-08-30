-- +goose Up
DROP TABLE IF EXISTS job_queue;

-- +goose Down
-- job_queue is not recreated: the queue it served was replaced by River.
