-- +goose Up
ALTER TABLE notifications ALTER COLUMN type DROP NOT NULL;

-- +goose Down
UPDATE notifications SET type = '' WHERE type IS NULL;
ALTER TABLE notifications ALTER COLUMN type SET NOT NULL;
