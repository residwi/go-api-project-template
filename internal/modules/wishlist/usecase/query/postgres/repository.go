package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ query.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListItemsForUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Item, error) {
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

	stmt := fmt.Sprintf(
		`SELECT wi.id, wi.wishlist_id, wi.product_id, wi.created_at
		FROM wishlist_items wi
		JOIN wishlists w ON w.id = wi.wishlist_id
		WHERE %s
		ORDER BY wi.created_at DESC, wi.id DESC
		LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("listing wishlist items: %w", err)
	}

	items, err := pgx.CollectRows(rows, scanItem)
	if err != nil {
		return nil, fmt.Errorf("listing wishlist items: %w", err)
	}

	return items, nil
}

func scanItem(row pgx.CollectableRow) (domain.Item, error) {
	var item domain.Item
	err := row.Scan(&item.ID, &item.WishlistID, &item.ProductID, &item.CreatedAt)
	return item, err
}
