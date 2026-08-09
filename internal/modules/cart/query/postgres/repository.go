package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/modules/cart/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

// Cart's own columns only. Joining products here would put product's lifecycle
// rule (deleted_at IS NULL) inside cart's query; the reader fills the rest
// through ProductLookup.
func scanCartItem(row pgx.CollectableRow) (domain.Item, error) {
	var item domain.Item
	err := row.Scan(&item.ID, &item.CartID, &item.ProductID, &item.Quantity,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return domain.Item{}, err
	}
	return item, nil
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error) {
	db := database.DB(ctx, r.pool)

	var c domain.Cart
	err := db.QueryRow(ctx,
		`SELECT id, user_id, created_at, updated_at FROM carts WHERE user_id = $1`,
		userID,
	).Scan(&c.ID, &c.UserID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &domain.Cart{UserID: userID, Items: []domain.Item{}}, nil
		}
		return nil, fmt.Errorf("getting cart: %w", err)
	}

	rows, err := db.Query(ctx,
		`SELECT id, cart_id, product_id, quantity, created_at, updated_at
		FROM cart_items
		WHERE cart_id = $1
		ORDER BY created_at`,
		c.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting cart items: %w", err)
	}
	c.Items, err = pgx.CollectRows(rows, scanCartItem)
	if err != nil {
		return nil, fmt.Errorf("scanning cart item: %w", err)
	}

	return &c, nil
}
