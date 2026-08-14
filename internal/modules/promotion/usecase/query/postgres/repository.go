package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

func scanPromotion(row pgx.CollectableRow) (domain.Promotion, error) {
	var p domain.Promotion
	err := row.Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListAdmin(ctx context.Context, params query.Params) ([]domain.Promotion, int, error) {
	db := database.DB(ctx, r.pool)

	var total int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM promotions`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting promotions: %w", err)
	}

	rows, err := db.Query(
		ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		params.Limit(),
		params.Offset(),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing promotions: %w", err)
	}
	promotions, err := pgx.CollectRows(rows, scanPromotion)
	if err != nil {
		return nil, 0, fmt.Errorf("listing promotions: %w", err)
	}

	return promotions, total, nil
}
