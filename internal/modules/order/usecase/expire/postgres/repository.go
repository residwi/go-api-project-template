package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/expire"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ expire.Repository = (*Repository)(nil)

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

func (r *Repository) GetExpiredOrders(ctx context.Context, limit int) ([]domain.Order, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE status = 'awaiting_payment' AND created_at < NOW() - INTERVAL '30 minutes'
		ORDER BY created_at LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting expired orders: %w", err)
	}
	orders, err := pgx.CollectRows(rows, scanOrderSummary)
	if err != nil {
		return nil, fmt.Errorf("getting expired orders: %w", err)
	}
	return orders, nil
}

func (r *Repository) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(
		ctx,
		`SELECT oi.id, oi.order_id, oi.product_id, oi.product_name, oi.price, oi.quantity, oi.subtotal, oi.created_at, o.currency
		FROM order_items oi JOIN orders o ON o.id = oi.order_id
		WHERE oi.order_id = $1 ORDER BY oi.created_at`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing order items: %w", err)
	}
	items, err := pgx.CollectRows(rows, scanItem)
	if err != nil {
		return nil, fmt.Errorf("listing order items: %w", err)
	}
	return items, nil
}

func scanItem(row pgx.CollectableRow) (domain.Item, error) {
	var item domain.Item
	var price, subtotal int64
	var currency string
	err := row.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.ProductName,
		&price, &item.Quantity, &subtotal, &item.CreatedAt, &currency)
	if err != nil {
		return item, err
	}
	item.Price = money.New(price, currency)
	item.Subtotal = money.New(subtotal, currency)
	return item, nil
}
