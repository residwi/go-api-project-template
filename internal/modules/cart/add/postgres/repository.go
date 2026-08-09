package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/add"
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

// CountAndHasItem answers both in one round-trip, so Execute enforces the
// distinct-item cap without a second query.
func (r *Repository) CountAndHasItem(ctx context.Context, cartID, productID uuid.UUID) (int, bool, error) {
	db := database.DB(ctx, r.pool)
	var (
		count      int
		hasProduct bool
	)
	err := db.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE product_id = $2) > 0 FROM cart_items WHERE cart_id = $1`,
		cartID, productID,
	).Scan(&count, &hasProduct)
	if err != nil {
		return 0, false, fmt.Errorf("counting cart items: %w", err)
	}
	return count, hasProduct, nil
}

func (r *Repository) AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`INSERT INTO cart_items (cart_id, product_id, quantity) VALUES ($1, $2, $3)
		ON CONFLICT (cart_id, product_id) DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity`,
		cartID, productID, qty,
	)
	if err != nil {
		return fmt.Errorf("adding cart item: %w", err)
	}
	return nil
}
