-- +goose Up
-- Cross-module cascades are unreachable: users and products are soft-deleted
-- (UPDATE ... SET deleted_at), so these ON DELETE CASCADE clauses never fire.
-- They also make the database perform cross-module writes that no Go port
-- describes -- deleting a product would reach into cart's and wishlist's
-- tables. Recreate each constraint without the cascade; the reference itself
-- stays as a monolith-level integrity backstop.
ALTER TABLE carts
    DROP CONSTRAINT carts_user_id_fkey,
    ADD CONSTRAINT carts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE cart_items
    DROP CONSTRAINT cart_items_product_id_fkey,
    ADD CONSTRAINT cart_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES products(id);

ALTER TABLE wishlists
    DROP CONSTRAINT wishlists_user_id_fkey,
    ADD CONSTRAINT wishlists_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE wishlist_items
    DROP CONSTRAINT wishlist_items_product_id_fkey,
    ADD CONSTRAINT wishlist_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES products(id);

ALTER TABLE notifications
    DROP CONSTRAINT notifications_user_id_fkey,
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE notification_jobs
    DROP CONSTRAINT notification_jobs_user_id_fkey,
    ADD CONSTRAINT notification_jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

-- +goose Down
ALTER TABLE carts
    DROP CONSTRAINT carts_user_id_fkey,
    ADD CONSTRAINT carts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE cart_items
    DROP CONSTRAINT cart_items_product_id_fkey,
    ADD CONSTRAINT cart_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;

ALTER TABLE wishlists
    DROP CONSTRAINT wishlists_user_id_fkey,
    ADD CONSTRAINT wishlists_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE wishlist_items
    DROP CONSTRAINT wishlist_items_product_id_fkey,
    ADD CONSTRAINT wishlist_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_user_id_fkey,
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE notification_jobs
    DROP CONSTRAINT notification_jobs_user_id_fkey,
    ADD CONSTRAINT notification_jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
