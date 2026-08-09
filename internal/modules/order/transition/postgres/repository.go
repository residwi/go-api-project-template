package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/transition"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ transition.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Apply(ctx context.Context, id uuid.UUID, t domain.Transition) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	// The flags ride along in the same CAS so they cannot disagree with the status.
	// OR keeps them monotonic: a transition that does not set one leaves it alone.
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1,
		        stock_deducted = stock_deducted OR $4,
		        stock_reversed = stock_reversed OR $5
		WHERE id = $2 AND status = ANY($3) RETURNING id`,
		t.To, id, t.From, t.SetsStockDeducted, t.SetsStockReversed,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("applying order transition: %w", err)
	}
	return nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2 AND status = $3 RETURNING id`,
		to, id, from,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating order status: %w", err)
	}
	return nil
}
