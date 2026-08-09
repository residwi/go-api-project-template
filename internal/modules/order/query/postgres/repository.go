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
	"github.com/residwi/go-api-project-template/internal/modules/order/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ query.Repository = (*Repository)(nil)

type amountColumns struct {
	subtotal int64
	discount int64
	total    int64
	currency string
}

func (a amountColumns) assignTo(o *domain.Order) {
	o.Subtotal = money.New(a.subtotal, a.currency)
	o.Discount = money.New(a.discount, a.currency)
	o.Total = money.New(a.total, a.currency)
}

func scanOrder(row pgx.CollectableRow) (domain.Order, error) {
	var o domain.Order
	var idempotencyKey, notes *string
	var amt amountColumns
	err := row.Scan(&o.ID, &o.UserID, &idempotencyKey, &o.Status,
		&amt.subtotal, &amt.discount, &amt.total,
		&o.CouponCode, &amt.currency, &o.ShippingAddress, &o.BillingAddress,
		&notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return o, err
	}
	amt.assignTo(&o)
	if idempotencyKey != nil {
		o.IdempotencyKey = *idempotencyKey
	}
	if notes != nil {
		o.Notes = *notes
	}
	return o, nil
}

// scanItem expects the parent order's currency as the last column: order_items
// has none of its own, so every query feeding this joins orders for it.
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
	var amt amountColumns
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &idempotencyKey, &requestHash, &o.Status,
		&amt.subtotal, &amt.discount, &amt.total,
		&o.CouponCode, &amt.currency, &o.ShippingAddress, &o.BillingAddress,
		&notes, &o.StockDeducted, &o.StockReversed, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by id: %w", err)
	}
	amt.assignTo(&o)
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

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Order, error) {
	db := database.DB(ctx, r.pool)

	args := []any{userID}
	where := "user_id = $1"
	argIdx := 2

	if cursor.Cursor != "" {
		var err error
		where, args, argIdx, err = database.KeysetCursor(where, args, argIdx, "created_at, id", cursor.Cursor)
		if err != nil {
			return nil, err
		}
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, created_at, updated_at
		FROM orders WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing user orders: %w", err)
	}
	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, fmt.Errorf("listing user orders: %w", err)
	}

	return orders, nil
}

func (r *Repository) ListAdmin(ctx context.Context, params query.AdminListParams) ([]domain.Order, int, error) {
	db := database.DB(ctx, r.pool)

	where := "1=1"
	args := []any{}
	argIdx := 1

	if params.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, params.Status)
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM orders WHERE " + where
	if err := db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting orders: %w", err)
	}

	q := fmt.Sprintf(
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, created_at, updated_at
		FROM orders WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.Limit(), params.Offset())

	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing orders: %w", err)
	}
	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, 0, fmt.Errorf("listing orders: %w", err)
	}

	return orders, total, nil
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

// HasDeliveredOrder binds to the specific orderID, not "some delivered order
// for this product": otherwise a review could be filed against an order that
// is not the reviewer's or never contained the product.
func (r *Repository) HasDeliveredOrder(ctx context.Context, p query.DeliveredPurchaseParams) (bool, error) {
	db := database.DB(ctx, r.pool)
	var exists bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM order_items oi
			JOIN orders o ON o.id = oi.order_id
			WHERE o.user_id = $1 AND o.id = $2 AND oi.product_id = $3 AND o.status = 'delivered'
		)`,
		p.UserID, p.OrderID, p.ProductID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking delivered order: %w", err)
	}
	return exists, nil
}
