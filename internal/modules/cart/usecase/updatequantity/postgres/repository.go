package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ updatequantity.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	db := database.DB(ctx, r.pool)
	var cartID uuid.UUID
	err := db.QueryRow(ctx,
		`INSERT INTO carts (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id`,
		userID,
	).Scan(&cartID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("get or create cart: %w", err)
	}
	return cartID, nil
}

func (r *Repository) UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE cart_items SET quantity = $1 WHERE cart_id = $2 AND product_id = $3`,
		qty, cartID, productID,
	)
	if err != nil {
		return fmt.Errorf("updating cart item quantity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
