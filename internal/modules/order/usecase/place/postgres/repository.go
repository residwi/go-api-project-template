package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/place"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ place.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, order *domain.Order) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, coupon_code, currency, shipping_address, billing_address, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`,
		order.UserID,
		order.IdempotencyKey,
		order.RequestHash,
		order.Status,
		order.Subtotal.Amount,
		order.Discount.Amount,
		order.Total.Amount,
		order.CouponCode,
		order.Total.Currency,
		order.ShippingAddress,
		order.BillingAddress,
		order.Notes,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating order: %w", err)
	}
	return nil
}

func (r *Repository) CreateItems(ctx context.Context, items []domain.Item) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)

	const orderItemColumns = 6
	placeholders := make([]string, len(items))
	args := make([]any, 0, len(items)*orderItemColumns)
	for i, item := range items {
		base := i * orderItemColumns
		parts := make([]string, orderItemColumns)

		for j := range orderItemColumns {
			parts[j] = fmt.Sprintf("$%d", base+j+1)
		}

		placeholders[i] = "(" + strings.Join(parts, ",") + ")"

		args = append(
			args,
			item.OrderID,
			item.ProductID,
			item.ProductName,
			item.Price.Amount,
			item.Quantity,
			item.Subtotal.Amount,
		)
	}

	rows, err := db.Query(ctx,
		`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal) VALUES `+
			strings.Join(placeholders, ",")+` RETURNING id, created_at`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("creating order items: %w", err)
	}

	type generated struct {
		ID        uuid.UUID
		CreatedAt time.Time
	}
	gen, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (generated, error) {
		var g generated
		return g, row.Scan(&g.ID, &g.CreatedAt)
	})
	if err != nil {
		return fmt.Errorf("scanning created order items: %w", err)
	}
	if len(gen) != len(items) {
		return fmt.Errorf("creating order items: expected %d rows, got %d", len(items), len(gen))
	}
	for i := range gen {
		items[i].ID = gen[i].ID               //nolint:gosec // len(gen) == len(items) is checked above
		items[i].CreatedAt = gen[i].CreatedAt //nolint:gosec // len(gen) == len(items) is checked above
	}
	return nil
}

func (r *Repository) GetByUserIDAndIdempotencyKey(
	ctx context.Context,
	userID uuid.UUID,
	key string,
) (*domain.Order, error) {
	db := database.DB(ctx, r.pool)
	var o domain.Order
	var idempotencyKey, requestHash, notes *string
	var subtotal, discount, total int64
	var currency string
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE user_id = $1 AND idempotency_key = $2`, userID, key,
	).Scan(&o.ID, &o.UserID, &idempotencyKey, &requestHash, &o.Status,
		&subtotal, &discount, &total,
		&o.CouponCode, &currency, &o.ShippingAddress, &o.BillingAddress,
		&notes, &o.StockDeducted, &o.StockReversed, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by idempotency key: %w", err)
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

func (r *Repository) UpdateTotals(ctx context.Context, id uuid.UUID, discount, total int64) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE orders SET discount_amount = $1, total_amount = $2 WHERE id = $3`,
		discount, total, id,
	)
	if err != nil {
		return fmt.Errorf("updating order totals: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
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
