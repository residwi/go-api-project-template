package cart

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error)
	AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error
	Clear(ctx context.Context, cartID uuid.UUID) error
	CountItems(ctx context.Context, cartID uuid.UUID) (int, error)
	GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
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

func (r *PostgresRepository) GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error) {
	db := database.DB(ctx, r.pool)

	var c Cart
	err := db.QueryRow(ctx,
		`SELECT id, user_id, created_at, updated_at FROM carts WHERE user_id = $1`,
		userID,
	).Scan(&c.ID, &c.UserID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Cart{UserID: userID, Items: []Item{}}, nil
		}
		return nil, fmt.Errorf("getting cart: %w", err)
	}

	rows, err := db.Query(ctx,
		`SELECT ci.id, ci.cart_id, ci.product_id, ci.quantity, ci.created_at, ci.updated_at,
		        p.name, p.price, p.currency, (p.stock_quantity - p.reserved_quantity), p.status
		FROM cart_items ci
		JOIN products p ON p.id = ci.product_id AND p.deleted_at IS NULL
		WHERE ci.cart_id = $1
		ORDER BY ci.created_at`,
		c.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting cart items: %w", err)
	}
	defer rows.Close()

	c.Items = []Item{}
	for rows.Next() {
		var item Item
		var cp Product
		if err := rows.Scan(
			&item.ID, &item.CartID, &item.ProductID, &item.Quantity, &item.CreatedAt, &item.UpdatedAt,
			&cp.Name, &cp.Price, &cp.Currency, &cp.Stock, &cp.Status,
		); err != nil {
			return nil, fmt.Errorf("scanning cart item: %w", err)
		}
		item.Product = &cp
		c.Items = append(c.Items, item)
	}

	return &c, nil
}

func (r *PostgresRepository) AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
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

func (r *PostgresRepository) UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE cart_items SET quantity = $1 WHERE cart_id = $2 AND product_id = $3`,
		qty, cartID, productID,
	)
	if err != nil {
		return fmt.Errorf("updating cart item quantity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2`,
		cartID, productID,
	)
	if err != nil {
		return fmt.Errorf("removing cart item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Clear(ctx context.Context, cartID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`DELETE FROM cart_items WHERE cart_id = $1`,
		cartID,
	)
	if err != nil {
		return fmt.Errorf("clearing cart: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CountItems(ctx context.Context, cartID uuid.UUID) (int, error) {
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

func (r *PostgresRepository) GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	db := database.DB(ctx, r.pool)
	var cartID uuid.UUID
	err := db.QueryRow(ctx,
		`SELECT id FROM carts WHERE user_id = $1 FOR UPDATE`,
		userID,
	).Scan(&cartID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, core.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("locking cart: %w", err)
	}
	return cartID, nil
}
