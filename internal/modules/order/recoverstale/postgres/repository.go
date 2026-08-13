package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/recoverstale"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ recoverstale.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func scanOrderSummary(row pgx.CollectableRow) (domain.Order, error) {
	var o domain.Order
	var idempotencyKey *string
	var subtotal, discount, total int64
	var currency string
	err := row.Scan(&o.ID, &o.UserID, &idempotencyKey, &o.Status,
		&subtotal, &discount, &total,
		&o.CouponCode, &currency, &o.StockDeducted, &o.StockReversed,
		&o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return o, err
	}
	o.Subtotal = money.New(subtotal, currency)
	o.Discount = money.New(discount, currency)
	o.Total = money.New(total, currency)
	if idempotencyKey != nil {
		o.IdempotencyKey = *idempotencyKey
	}
	return o, nil
}

func (r *Repository) GetStaleProcessingOrders(
	ctx context.Context,
	threshold time.Duration,
	limit int,
) ([]domain.Order, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE status = 'payment_processing' AND updated_at < NOW() - $1::interval
		ORDER BY updated_at LIMIT $2`, threshold.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting stale processing orders: %w", err)
	}
	orders, err := pgx.CollectRows(rows, scanOrderSummary)
	if err != nil {
		return nil, fmt.Errorf("getting stale processing orders: %w", err)
	}
	return orders, nil
}
