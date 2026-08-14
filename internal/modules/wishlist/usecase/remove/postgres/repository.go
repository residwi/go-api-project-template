package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/remove"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ remove.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM wishlist_items wi
		USING wishlists w
		WHERE wi.wishlist_id = w.id AND w.user_id = $1 AND wi.product_id = $2`,
		userID, productID,
	)
	if err != nil {
		return fmt.Errorf("removing wishlist item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
