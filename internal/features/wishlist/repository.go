package wishlist

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
	AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error
	RemoveItem(ctx context.Context, wishlistID, productID uuid.UUID) error
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
		return fmt.Errorf("adding wishlist item: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RemoveItem(ctx context.Context, wishlistID, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM wishlist_items WHERE wishlist_id = $1 AND product_id = $2`,
		wishlistID, productID,
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

	var wishlistID uuid.UUID
	err := db.QueryRow(ctx,
		`SELECT id FROM wishlists WHERE user_id = $1`, userID,
	).Scan(&wishlistID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []Item{}, nil
		}
		return nil, fmt.Errorf("getting wishlist: %w", err)
	}

	args := []any{wishlistID}
	where := "wi.wishlist_id = $1"
	argIdx := 2

	if cursor.Cursor != "" {
		cursorCreatedAt, cursorID, cursorErr := core.DecodeCursor(cursor.Cursor)
		if cursorErr != nil {
			return nil, fmt.Errorf("%w: invalid cursor", core.ErrBadRequest)
		}
		where += fmt.Sprintf(" AND (wi.created_at, wi.id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursorCreatedAt, cursorID)
		argIdx += 2
	}

	query := fmt.Sprintf(
		`SELECT wi.id, wi.wishlist_id, wi.product_id, wi.created_at
		FROM wishlist_items wi
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
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.WishlistID, &item.ProductID, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning wishlist item: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
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
