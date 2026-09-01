package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ order.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	db := database.PrimaryDB(ctx, r.db)
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
			return nil, errs.ErrNotFound
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
	db := database.PrimaryDB(ctx, r.db)

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

func (r *Repository) ListAdmin(ctx context.Context, params order.AdminListParams) ([]domain.Order, int, error) {
	db := database.ReplicaDB(ctx, r.db)

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
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) HasDeliveredOrder(ctx context.Context, p order.DeliveredPurchaseParams) (bool, error) {
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) Create(ctx context.Context, o *domain.Order) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, coupon_code, currency, shipping_address, billing_address, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`,
		o.UserID,
		o.IdempotencyKey,
		o.RequestHash,
		o.Status,
		o.Subtotal.Amount,
		o.Discount.Amount,
		o.Total.Amount,
		o.CouponCode,
		o.Total.Currency,
		o.ShippingAddress,
		o.BillingAddress,
		o.Notes,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return errs.ErrConflict
		}
		return fmt.Errorf("creating order: %w", err)
	}
	return nil
}

func (r *Repository) CreateItems(ctx context.Context, items []domain.Item) error {
	if len(items) == 0 {
		return nil
	}
	db := database.PrimaryDB(ctx, r.db)

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
	db := database.PrimaryDB(ctx, r.db)
	var o domain.Order
	var idempotencyKey, requestHash, notes *string
	var amt amountColumns
	err := db.QueryRow(ctx,
		`SELECT id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount,
		        coupon_code, currency, shipping_address, billing_address, notes, stock_deducted, stock_reversed, created_at, updated_at
		FROM orders WHERE user_id = $1 AND idempotency_key = $2`, userID, key,
	).Scan(&o.ID, &o.UserID, &idempotencyKey, &requestHash, &o.Status,
		&amt.subtotal, &amt.discount, &amt.total,
		&o.CouponCode, &amt.currency, &o.ShippingAddress, &o.BillingAddress,
		&notes, &o.StockDeducted, &o.StockReversed, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("getting order by idempotency key: %w", err)
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

func (r *Repository) UpdateTotals(ctx context.Context, id uuid.UUID, discount, total int64) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx,
		`UPDATE orders SET discount_amount = $1, total_amount = $2 WHERE id = $3`,
		discount, total, id,
	)
	if err != nil {
		return fmt.Errorf("updating order totals: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *Repository) GetExpiredOrders(ctx context.Context, limit int) ([]domain.Order, error) {
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) GetStaleProcessingOrders(
	ctx context.Context,
	threshold time.Duration,
	limit int,
) ([]domain.Order, error) {
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) Apply(ctx context.Context, id uuid.UUID, t domain.Transition) error {
	db := database.PrimaryDB(ctx, r.db)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1,
		        stock_deducted = stock_deducted OR $4,
		        stock_reversed = stock_reversed OR $5
		WHERE id = $2 AND status = ANY($3) RETURNING id`,
		t.To, id, t.From, t.DeductsStock(), t.ReversesStock(),
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrConflict
		}
		return fmt.Errorf("applying order transition: %w", err)
	}
	return nil
}

func scanOrderSummary(row pgx.CollectableRow) (domain.Order, error) {
	var o domain.Order
	var idempotencyKey *string
	var amt amountColumns
	err := row.Scan(&o.ID, &o.UserID, &idempotencyKey, &o.Status,
		&amt.subtotal, &amt.discount, &amt.total,
		&o.CouponCode, &amt.currency, &o.StockDeducted, &o.StockReversed,
		&o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return o, err
	}
	amt.assignTo(&o)
	if idempotencyKey != nil {
		o.IdempotencyKey = *idempotencyKey
	}
	return o, nil
}

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
