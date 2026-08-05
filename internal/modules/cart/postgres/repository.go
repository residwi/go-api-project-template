package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// Cart's own columns only. Joining products here would put product's lifecycle
// rule (deleted_at IS NULL) inside cart's query; the service fills the rest
// through ProductLookup.
func scanCartItem(row pgx.CollectableRow) (cart.Item, error) {
	var item cart.Item
	err := row.Scan(&item.ID, &item.CartID, &item.ProductID, &item.Quantity,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return cart.Item{}, err
	}
	return item, nil
}

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

func (r *Repository) GetCart(ctx context.Context, userID uuid.UUID) (*cart.Cart, error) {
	db := database.DB(ctx, r.pool)

	var c cart.Cart
	err := db.QueryRow(ctx,
		`SELECT id, user_id, created_at, updated_at FROM carts WHERE user_id = $1`,
		userID,
	).Scan(&c.ID, &c.UserID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &cart.Cart{UserID: userID, Items: []cart.Item{}}, nil
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

func (r *Repository) RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2`,
		cartID, productID,
	)
	if err != nil {
		return fmt.Errorf("removing cart item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *Repository) Clear(ctx context.Context, userID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`DELETE FROM cart_items WHERE cart_id = (SELECT id FROM carts WHERE user_id = $1)`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("clearing cart: %w", err)
	}
	return nil
}

func (r *Repository) CountItems(ctx context.Context, cartID uuid.UUID) (int, error) {
	db := database.DB(ctx, r.pool)
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM cart_items WHERE cart_id = $1`,
		cartID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting cart items: %w", err)
	}
	return count, nil
}

// CountAndHasItem answers both in one round-trip, so AddItem enforces the
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

func (r *Repository) GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	db := database.DB(ctx, r.pool)
	var cartID uuid.UUID
	err := db.QueryRow(ctx,
		`SELECT id FROM carts WHERE user_id = $1 FOR UPDATE`,
		userID,
	).Scan(&cartID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperror.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("locking cart: %w", err)
	}
	return cartID, nil
}
