package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/modules/review"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ review.Repository = (*Repository)(nil)

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
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, rv *domain.Review) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(ctx,
		`INSERT INTO reviews (user_id, product_id, order_id, rating, title, body, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		rv.UserID, rv.ProductID, rv.OrderID, rv.Rating, rv.Title, rv.Body, rv.Status,
	).Scan(&rv.ID, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return errs.ErrConflict
		}
		return fmt.Errorf("creating review: %w", err)
	}
	return nil
}

func (r *Repository) HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	db := database.PrimaryDB(ctx, r.db)
	var exists bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reviews WHERE user_id = $1 AND product_id = $2)`,
		userID, productID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking user review: %w", err)
	}
	return exists, nil
}

func (r *Repository) ListByProduct(
	ctx context.Context,
	productID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Review, error) {
	db := database.PrimaryDB(ctx, r.db)

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
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}
