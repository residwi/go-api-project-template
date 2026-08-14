package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/update"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ update.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Promotion, error) {
	db := database.DB(ctx, r.pool)
	var p domain.Promotion
	err := db.QueryRow(
		ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting promotion by id: %w", err)
	}
	return &p, nil
}

func (r *Repository) Update(ctx context.Context, promo *domain.Promotion) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(
		ctx,
		`UPDATE promotions SET code=$1, type=$2, value=$3, min_order_amount=$4, max_discount=$5, max_uses=$6, starts_at=$7, expires_at=$8, active=$9
		WHERE id = $10`,
		promo.Code,
		promo.Type,
		promo.Value,
		promo.MinOrderAmount,
		promo.MaxDiscount,
		promo.MaxUses,
		promo.StartsAt,
		promo.ExpiresAt,
		promo.Active,
		promo.ID,
	)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating promotion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
