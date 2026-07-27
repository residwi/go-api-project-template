package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/review"
)

func scanReview(row pgx.CollectableRow) (review.Review, error) {
	var rv review.Review
	err := row.Scan(&rv.ID, &rv.UserID, &rv.ProductID, &rv.OrderID, &rv.Rating, &rv.Title, &rv.Body, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	return rv, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, review *review.Review) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO reviews (user_id, product_id, order_id, rating, title, body, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		review.UserID, review.ProductID, review.OrderID, review.Rating, review.Title, review.Body, review.Status,
	).Scan(&review.ID, &review.CreatedAt, &review.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating review: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*review.Review, error) {
	db := database.DB(ctx, r.pool)
	var rv review.Review
	err := db.QueryRow(ctx,
		`SELECT id, user_id, product_id, order_id, rating, title, body, status, created_at, updated_at
		FROM reviews WHERE id = $1`, id,
	).Scan(&rv.ID, &rv.UserID, &rv.ProductID, &rv.OrderID, &rv.Rating, &rv.Title, &rv.Body, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting review by id: %w", err)
	}
	return &rv, nil
}

func (r *Repository) ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]review.Review, error) {
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

	query := fmt.Sprintf(
		`SELECT id, user_id, product_id, order_id, rating, title, body, status, created_at, updated_at
		FROM reviews WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing product reviews: %w", err)
	}
	reviews, err := pgx.CollectRows(rows, scanReview)
	if err != nil {
		return nil, fmt.Errorf("listing product reviews: %w", err)
	}

	return reviews, nil
}

func (r *Repository) GetStats(ctx context.Context, productID uuid.UUID) (review.Stats, error) {
	db := database.DB(ctx, r.pool)
	var stats review.Stats
	err := db.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating), 0), COUNT(*)
		FROM reviews WHERE product_id = $1 AND status = 'published'`, productID,
	).Scan(&stats.AverageRating, &stats.TotalReviews)
	if err != nil {
		return stats, fmt.Errorf("getting review stats: %w", err)
	}
	return stats, nil
}

func (r *Repository) HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	db := database.DB(ctx, r.pool)
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

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
