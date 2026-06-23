package wishlist

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error
	RemoveItem(ctx context.Context, userID, productID uuid.UUID) error
	GetItems(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Item, error)
	HasItem(ctx context.Context, wishlistID, productID uuid.UUID) (bool, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
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

func (r *PostgresRepository) AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`INSERT INTO wishlist_items (wishlist_id, product_id) VALUES ($1, $2)
		ON CONFLICT (wishlist_id, product_id) DO NOTHING`,
		wishlistID, productID,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: product not found", core.ErrNotFound)
		}
		return fmt.Errorf("adding wishlist item: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
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
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetItems(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Item, error) {
	db := database.DB(ctx, r.pool)

	args := []any{userID}
	where := "w.user_id = $1"
	argIdx := 2

	if cursor.Cursor != "" {
		var err error
		where, args, argIdx, err = database.KeysetCursor(where, args, argIdx, "wi.created_at, wi.id", cursor.Cursor)
		if err != nil {
			return nil, err
		}
	}

	query := fmt.Sprintf(
		`SELECT wi.id, wi.wishlist_id, wi.product_id, wi.created_at
		FROM wishlist_items wi
		JOIN wishlists w ON w.id = wi.wishlist_id
		WHERE %s
		ORDER BY wi.created_at DESC, wi.id DESC
		LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing wishlist items: %w", err)
	}

	items, err := pgx.CollectRows(rows, scanItem)
	if err != nil {
		return nil, fmt.Errorf("listing wishlist items: %w", err)
	}

	return items, nil
}

func scanItem(row pgx.CollectableRow) (Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.WishlistID, &item.ProductID, &item.CreatedAt)
	return item, err
}

func (r *PostgresRepository) HasItem(ctx context.Context, wishlistID, productID uuid.UUID) (bool, error) {
	db := database.DB(ctx, r.pool)
	var exists bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM wishlist_items WHERE wishlist_id = $1 AND product_id = $2)`,
		wishlistID, productID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking wishlist item: %w", err)
	}
	return exists, nil
}
