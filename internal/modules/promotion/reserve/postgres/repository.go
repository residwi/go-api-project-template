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
	"github.com/residwi/go-api-project-template/internal/modules/promotion/reserve"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ reserve.Repository = (*Repository)(nil)

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
