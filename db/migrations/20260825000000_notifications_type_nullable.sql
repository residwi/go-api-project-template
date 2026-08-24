-- +goose Up
ALTER TABLE notifications ALTER COLUMN type DROP NOT NULL;

-- +goose Down
ALTER TABLE notifications ALTER COLUMN type SET NOT NULL;
