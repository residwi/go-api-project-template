package promotion

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
	Create(ctx context.Context, promo *Promotion) error
	GetByID(ctx context.Context, id uuid.UUID) (*Promotion, error)
	GetByCode(ctx context.Context, code string) (*Promotion, error)
	Update(ctx context.Context, promo *Promotion) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListAdmin(ctx context.Context, params ListParams) ([]Promotion, int, error)
	ApplyPromotion(ctx context.Context, id uuid.UUID) error
	ReleasePromotion(ctx context.Context, id uuid.UUID) error
	CreateUsage(ctx context.Context, usage *CouponUsage) error
	DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*CouponUsage, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, promo *Promotion) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO promotions (code, type, value, min_order_amount, max_discount, max_uses, starts_at, expires_at, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, used_count, created_at, updated_at`,
		promo.Code, promo.Type, promo.Value, promo.MinOrderAmount,
		promo.MaxDiscount, promo.MaxUses, promo.StartsAt, promo.ExpiresAt, promo.Active,
	).Scan(&promo.ID, &promo.UsedCount, &promo.CreatedAt, &promo.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating promotion: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Promotion, error) {
	db := database.DB(ctx, r.pool)
	var p Promotion
	err := db.QueryRow(ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions WHERE id = $1`, id,
	).Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting promotion by id: %w", err)
	}
	return &p, nil
}

func (r *PostgresRepository) GetByCode(ctx context.Context, code string) (*Promotion, error) {
	db := database.DB(ctx, r.pool)
	var p Promotion
	err := db.QueryRow(ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions WHERE code = $1`, code,
	).Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
		&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
		&p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting promotion by code: %w", err)
	}
	return &p, nil
}

func (r *PostgresRepository) Update(ctx context.Context, promo *Promotion) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE promotions SET code=$1, type=$2, value=$3, min_order_amount=$4, max_discount=$5, max_uses=$6, starts_at=$7, expires_at=$8, active=$9
		WHERE id = $10`,
		promo.Code, promo.Type, promo.Value, promo.MinOrderAmount,
		promo.MaxDiscount, promo.MaxUses, promo.StartsAt, promo.ExpiresAt, promo.Active, promo.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("updating promotion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting promotion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListAdmin(ctx context.Context, params ListParams) ([]Promotion, int, error) {
	db := database.DB(ctx, r.pool)

	var total int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM promotions`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting promotions: %w", err)
	}

	offset := (params.Page - 1) * params.PageSize
	rows, err := db.Query(ctx,
		`SELECT id, code, type, value, min_order_amount, max_discount, max_uses, used_count, starts_at, expires_at, active, created_at, updated_at
		FROM promotions ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		params.PageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing promotions: %w", err)
	}
	defer rows.Close()

	var promotions []Promotion
	for rows.Next() {
		var p Promotion
		if err := rows.Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MinOrderAmount,
			&p.MaxDiscount, &p.MaxUses, &p.UsedCount, &p.StartsAt, &p.ExpiresAt,
			&p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning promotion: %w", err)
		}
		promotions = append(promotions, p)
	}

	return promotions, total, nil
}

func (r *PostgresRepository) ApplyPromotion(ctx context.Context, id uuid.UUID) error {
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
		return core.ErrCouponExhausted
	}
	return nil
}

func (r *PostgresRepository) ReleasePromotion(ctx context.Context, id uuid.UUID) error {
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

func (r *PostgresRepository) CreateUsage(ctx context.Context, usage *CouponUsage) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO coupon_usages (coupon_id, user_id, order_id, discount)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		usage.CouponID, usage.UserID, usage.OrderID, usage.Discount,
	).Scan(&usage.ID, &usage.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating coupon usage: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*CouponUsage, error) {
	db := database.DB(ctx, r.pool)
	var usage CouponUsage
	err := db.QueryRow(ctx,
		`DELETE FROM coupon_usages WHERE order_id = $1 RETURNING coupon_id, discount`,
		orderID,
	).Scan(&usage.CouponID, &usage.Discount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("deleting coupon usage by order: %w", err)
	}
	return &usage, nil
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
