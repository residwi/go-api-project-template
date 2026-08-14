package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/lock"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ lock.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
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
