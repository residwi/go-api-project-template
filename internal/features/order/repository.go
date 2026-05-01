package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type AdminListParams struct {
	Page     int
	PageSize int
	Status   string
}

type Repository interface {
	Create(ctx context.Context, order *Order) error
	CreateItems(ctx context.Context, items []Item) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByUserIDAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Order, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]Order, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, fromStatus, toStatus Status) error
	UpdateStatusMulti(ctx context.Context, id uuid.UUID, toStatus Status, fromStatuses []Status) error
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error)
	GetExpiredOrders(ctx context.Context, limit int) ([]Order, error)
	GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]Order, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, order *Order) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, coupon_code, currency, shipping_address, billing_address, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`,
		order.UserID, order.IdempotencyKey, order.RequestHash, order.Status,
		order.SubtotalAmount, order.DiscountAmount, order.TotalAmount,
		order.CouponCode, order.Currency,
		order.ShippingAddress, order.BillingAddress, order.Notes,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating order: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateItems(ctx context.Context, items []Item) error {
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

		args = append(args, item.OrderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Subtotal)
	}

	rows, err := db.Query(ctx,
		`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal) VALUES `+
			strings.Join(placeholders, ",")+` RETURNING id, created_at`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("creating order items: %w", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		if err := rows.Scan(&items[i].ID, &items[i].CreatedAt); err != nil {
			return fmt.Errorf("scanning order item: %w", err)
		}
		i++
	}
	return rows.Err()
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	db := database.DB(ctx, r.pool)
	var o Order
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, created_at, updated_at
		FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &o.IdempotencyKey, &o.RequestHash, &o.Status,
		&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
		&o.CouponCode, &o.Currency, &o.ShippingAddress, &o.BillingAddress,
		&o.Notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by id: %w", err)
	}
	return &o, nil
}

func (r *PostgresRepository) GetByUserIDAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*Order, error) {
	db := database.DB(ctx, r.pool)
	var o Order
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, created_at, updated_at
		FROM orders WHERE user_id = $1 AND idempotency_key = $2`, userID, key,
	).Scan(&o.ID, &o.UserID, &o.IdempotencyKey, &o.RequestHash, &o.Status,
		&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
		&o.CouponCode, &o.Currency, &o.ShippingAddress, &o.BillingAddress,
		&o.Notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by idempotency key: %w", err)
	}
	return &o, nil
}

func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Order, error) {
	db := database.DB(ctx, r.pool)

	args := []any{userID}
	where := "user_id = $1"
	argIdx := 2

	if cursor.Cursor != "" {
		cursorTime, cursorID, err := core.DecodeCursor(cursor.Cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid cursor", core.ErrBadRequest)
		}
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursorTime, cursorID)
		argIdx += 2
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
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.IdempotencyKey, &o.Status,
			&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
			&o.CouponCode, &o.Currency, &o.ShippingAddress, &o.BillingAddress,
			&o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}

	return orders, nil
}

func (r *PostgresRepository) ListAdmin(ctx context.Context, params AdminListParams) ([]Order, int, error) {
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

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, created_at, updated_at
		FROM orders WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.IdempotencyKey, &o.Status,
			&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
			&o.CouponCode, &o.Currency, &o.ShippingAddress, &o.BillingAddress,
			&o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}

	return orders, total, nil
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, fromStatus, toStatus Status) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2 AND status = $3 RETURNING id`,
		toStatus, id, fromStatus,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return core.ErrConflict
		}
		return fmt.Errorf("updating order status: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateStatusMulti(ctx context.Context, id uuid.UUID, toStatus Status, fromStatuses []Status) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2 AND status = ANY($3) RETURNING id`,
		toStatus, id, fromStatuses,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return core.ErrConflict
		}
		return fmt.Errorf("updating order status multi: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, order_id, product_id, product_name, price, quantity, subtotal, created_at
		FROM order_items WHERE order_id = $1 ORDER BY created_at`, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing order items: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.ProductName,
			&item.Price, &item.Quantity, &item.Subtotal, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning order item: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresRepository) GetExpiredOrders(ctx context.Context, limit int) ([]Order, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, created_at, updated_at
		FROM orders WHERE status = 'awaiting_payment' AND created_at < NOW() - INTERVAL '30 minutes'
		ORDER BY created_at LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting expired orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		var idempotencyKey *string
		if err := rows.Scan(&o.ID, &o.UserID, &idempotencyKey, &o.Status,
			&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
			&o.CouponCode, &o.Currency, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning expired order: %w", err)
		}
		if idempotencyKey != nil {
			o.IdempotencyKey = *idempotencyKey
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func (r *PostgresRepository) GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]Order, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, created_at, updated_at
		FROM orders WHERE status = 'payment_processing' AND updated_at < NOW() - $1::interval
		ORDER BY updated_at LIMIT $2`, threshold.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting stale processing orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		var idempotencyKey *string
		if err := rows.Scan(&o.ID, &o.UserID, &idempotencyKey, &o.Status,
			&o.SubtotalAmount, &o.DiscountAmount, &o.TotalAmount,
			&o.CouponCode, &o.Currency, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning stale order: %w", err)
		}
		if idempotencyKey != nil {
			o.IdempotencyKey = *idempotencyKey
		}
		orders = append(orders, o)
	}
	return orders, nil
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
