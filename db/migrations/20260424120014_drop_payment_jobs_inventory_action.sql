-- +goose Up
-- The refund worker now recomputes release-vs-restock from the order snapshot at
-- process time (inventory owns that choice via inventory.Restore), so the
-- per-job inventory_action is dead state. Dropping the column also drops its
-- CHECK constraint.
ALTER TABLE payment_jobs DROP COLUMN inventory_action;

-- +goose Down
ALTER TABLE payment_jobs ADD COLUMN inventory_action VARCHAR(50);
ALTER TABLE payment_jobs ADD CONSTRAINT payment_jobs_inventory_action_check
    CHECK (inventory_action IS NULL OR inventory_action IN ('release', 'restock'));
