package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/modules/review/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ query.Repository = (*Repository)(nil)

func scanReview(row pgx.CollectableRow) (domain.Review, error) {
	var rv domain.Review
	err := row.Scan(
		&rv.ID,
		&rv.UserID,
		&rv.ProductID,
		&rv.OrderID,
		&rv.Rating,
		&rv.Title,
		&rv.Body,
		&rv.Status,
		&rv.CreatedAt,
		&rv.UpdatedAt,
	)
	return rv, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListByProduct(
	ctx context.Context,
	productID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Review, error) {
	db := database.DB(ctx, r.pool)

	args := []any{productID}
	where := "product_id = $1 AND status = 'published'"
	argIdx := 2

	if cursor.Cursor != "" {
		var err error
		where, args, argIdx, err = database.KeysetCursor(where, args, argIdx, "created_at, id", cursor.Cursor)
		if err != nil {
			return nil, err
		}
	}

	q := fmt.Sprintf(
		`SELECT id, user_id, product_id, order_id, rating, title, body, status, created_at, updated_at
		FROM reviews WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing product reviews: %w", err)
	}
	reviews, err := pgx.CollectRows(rows, scanReview)
	if err != nil {
		return nil, fmt.Errorf("listing product reviews: %w", err)
	}

	return reviews, nil
}

func (r *Repository) GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error) {
	db := database.DB(ctx, r.pool)
	var stats domain.Stats
	err := db.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating), 0), COUNT(*)
		FROM reviews WHERE product_id = $1 AND status = 'published'`, productID,
	).Scan(&stats.AverageRating, &stats.TotalReviews)
	if err != nil {
		return stats, fmt.Errorf("getting review stats: %w", err)
	}
	return stats, nil
}
