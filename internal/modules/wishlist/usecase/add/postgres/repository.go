package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/add"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ add.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	db := database.DB(ctx, r.pool)
	var wishlistID uuid.UUID
	err := db.QueryRow(ctx,
		`INSERT INTO wishlists (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id`,
		userID,
	).Scan(&wishlistID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("get or create wishlist: %w", err)
	}
	return wishlistID, nil
}

func (r *Repository) AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`INSERT INTO wishlist_items (wishlist_id, product_id) VALUES ($1, $2)
		ON CONFLICT (wishlist_id, product_id) DO NOTHING`,
		wishlistID, productID,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: product not found", apperror.ErrNotFound)
		}
		return fmt.Errorf("adding wishlist item: %w", err)
	}
	return nil
}
