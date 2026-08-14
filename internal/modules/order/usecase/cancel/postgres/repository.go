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
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/cancel"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ cancel.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	db := database.DB(ctx, r.pool)
	var o domain.Order
	var idempotencyKey, requestHash, notes *string
	var subtotal, discount, total int64
	var currency string
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &idempotencyKey, &requestHash, &o.Status,
		&subtotal, &discount, &total,
		&o.CouponCode, &currency, &o.ShippingAddress, &o.BillingAddress,
		&notes, &o.StockDeducted, &o.StockReversed, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by id: %w", err)
	}
	o.Subtotal = money.New(subtotal, currency)
	o.Discount = money.New(discount, currency)
	o.Total = money.New(total, currency)
	if idempotencyKey != nil {
		o.IdempotencyKey = *idempotencyKey
	}
	if requestHash != nil {
		o.RequestHash = *requestHash
	}
	if notes != nil {
		o.Notes = *notes
	}
	return &o, nil
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
