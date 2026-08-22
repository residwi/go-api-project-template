package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ promotion.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByCode(ctx context.Context, code string) (*domain.Promotion, error) {
	db := database.DB(ctx, r.pool)
	var p domain.Promotion
	err := db.QueryRow(
		ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions WHERE code = $1`,
		code,
	).Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting promotion by code: %w", err)
	}
	return &p, nil
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

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting promotion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func scanPromotion(row pgx.CollectableRow) (domain.Promotion, error) {
	var p domain.Promotion
	err := row.Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) ListAdmin(ctx context.Context, params promotion.AdminListParams) ([]domain.Promotion, int, error) {
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

func (r *Repository) ApplyPromotion(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE promotions SET used_count = used_count + 1
		WHERE id = $1 AND active = true AND (max_uses IS NULL OR used_count < max_uses)
		AND starts_at <= NOW() AND expires_at >= NOW()`,
		id,
	)
	if err != nil {
		return fmt.Errorf("applying promotion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrCouponExhausted
	}
	return nil
}

func (r *Repository) ReleasePromotion(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE promotions SET used_count = used_count - 1 WHERE id = $1 AND used_count > 0`,
		id,
	)
	if err != nil {
		return fmt.Errorf("releasing promotion: %w", err)
	}
	return nil
}

func (r *Repository) CreateUsage(ctx context.Context, usage *domain.CouponUsage) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO coupon_usages (coupon_id, user_id, order_id, discount)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		usage.CouponID, usage.UserID, usage.OrderID, usage.Discount,
	).Scan(&usage.ID, &usage.CreatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating coupon usage: %w", err)
	}
	return nil
}

func (r *Repository) DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.CouponUsage, error) {
	db := database.DB(ctx, r.pool)
	var usage domain.CouponUsage
	err := db.QueryRow(ctx,
		`DELETE FROM coupon_usages WHERE order_id = $1 RETURNING coupon_id, discount`,
		orderID,
	).Scan(&usage.CouponID, &usage.Discount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("deleting coupon usage by order: %w", err)
	}
	return &usage, nil
}
