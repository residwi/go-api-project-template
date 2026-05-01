package review

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
	Create(ctx context.Context, review *Review) error
	GetByID(ctx context.Context, id uuid.UUID) (*Review, error)
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor core.CursorPage) ([]Review, error)
	GetStats(ctx context.Context, productID uuid.UUID) (Stats, error)
	HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, review *Review) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO reviews (user_id, product_id, order_id, rating, title, body, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		review.UserID, review.ProductID, review.OrderID, review.Rating, review.Title, review.Body, review.Status,
	).Scan(&review.ID, &review.CreatedAt, &review.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating review: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Review, error) {
	db := database.DB(ctx, r.pool)
	var rv Review
	err := db.QueryRow(ctx,
		`SELECT id, user_id, product_id, order_id, rating, title, body, status, created_at, updated_at
		FROM reviews WHERE id = $1`, id,
	).Scan(&rv.ID, &rv.UserID, &rv.ProductID, &rv.OrderID, &rv.Rating, &rv.Title, &rv.Body, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting review by id: %w", err)
	}
	return &rv, nil
}

func (r *PostgresRepository) ListByProduct(ctx context.Context, productID uuid.UUID, cursor core.CursorPage) ([]Review, error) {
	db := database.DB(ctx, r.pool)

	args := []any{productID}
	where := "product_id = $1 AND status = 'published'"
	argIdx := 2

	if cursor.Cursor != "" {
		cursorTime, cursorID, err := core.DecodeCursor(cursor.Cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid cursor", core.ErrBadRequest)
		}
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursorTime, cursorID)
		argIdx += 2
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
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var rv Review
		if err := rows.Scan(&rv.ID, &rv.UserID, &rv.ProductID, &rv.OrderID, &rv.Rating, &rv.Title, &rv.Body, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning review: %w", err)
		}
		reviews = append(reviews, rv)
	}

	return reviews, nil
}

func (r *PostgresRepository) GetStats(ctx context.Context, productID uuid.UUID) (Stats, error) {
	db := database.DB(ctx, r.pool)
	var stats Stats
	err := db.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating), 0), COUNT(*)
		FROM reviews WHERE product_id = $1 AND status = 'published'`, productID,
	).Scan(&stats.AverageRating, &stats.TotalReviews)
	if err != nil {
		return stats, fmt.Errorf("getting review stats: %w", err)
	}
	return stats, nil
}

func (r *PostgresRepository) HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
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

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && contains(err.Error(), "duplicate key value violates unique constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
