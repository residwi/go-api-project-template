package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/create"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ create.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, promo *domain.Promotion) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(
		ctx,
		`INSERT INTO promotions (code, type, value, min_order_amount, max_discount, max_uses, starts_at, expires_at, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, used_count, created_at, updated_at`,
		promo.Code,
		promo.Type,
		promo.Value,
		promo.MinOrderAmount,
		promo.MaxDiscount,
		promo.MaxUses,
		promo.StartsAt,
		promo.ExpiresAt,
		promo.Active,
	).Scan(&promo.ID, &promo.UsedCount, &promo.CreatedAt, &promo.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating promotion: %w", err)
	}
	return nil
}
