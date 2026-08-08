package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/review/create"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ create.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, rv *domain.Review) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO reviews (user_id, product_id, order_id, rating, title, body, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		rv.UserID, rv.ProductID, rv.OrderID, rv.Rating, rv.Title, rv.Body, rv.Status,
	).Scan(&rv.ID, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating review: %w", err)
	}
	return nil
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
