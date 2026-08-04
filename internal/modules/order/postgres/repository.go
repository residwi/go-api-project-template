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
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// amountColumns is the orders table's three amount columns plus the single
// currency column all three share. The schema stores the currency once, so a
// scan reads it once and denominates all three money.Money values from it --
// this struct is the one place that fan-out lives.
type amountColumns struct {
	subtotal int64
	discount int64
	total    int64
	currency string
}

func (a amountColumns) assignTo(o *order.Order) {
	o.Subtotal = money.New(a.subtotal, a.currency)
	o.Discount = money.New(a.discount, a.currency)
	o.Total = money.New(a.total, a.currency)
}

func scanOrder(row pgx.CollectableRow) (order.Order, error) {
	var o order.Order
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

func scanOrderSummary(row pgx.CollectableRow) (order.Order, error) {
	var o order.Order
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

// scanItem expects the parent order's currency as the last column. order_items
// has no currency column of its own -- the currency belongs to the order, and
// storing it per row could only ever disagree with it -- so every query feeding
// this scanner joins orders for it. Without that the items would come back
// denominated in nothing and refuse to sum against the order's total.
func scanItem(row pgx.CollectableRow) (order.Item, error) {
	var item order.Item
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

func (r *Repository) Create(ctx context.Context, order *order.Order) error {
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
		// One currency column for all three amounts; Total's is authoritative
		// because it is what gets charged.
		//
		// This silently ignores whether Subtotal and Discount agree with it, which
		// would re-denominate them on the next read. Not a live defect: the sole
		// caller (order.Service.PlaceOrder) builds all three from subtotal's
		// currency, and Money makes a disagreement unrepresentable upstream of
		// here since the fold that produces subtotal already refuses mixed
		// currencies. Flagged rather than guarded because it is the one write in
		// the four Money features that could re-denominate an amount, so a second
		// caller would need to preserve that invariant itself.
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

func (r *Repository) CreateItems(ctx context.Context, items []order.Item) error {
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

		// Only the amounts are written: the item's currency is the order's, already
		// stored on the orders row, and the service guarantees they agree.
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

	// RETURNING yields one row per inserted item in insertion order; collect the
	// generated id/created_at and write them back onto the input items.
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

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*order.Order, error) {
	db := database.DB(ctx, r.pool)
	var o order.Order
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

func (r *Repository) GetByUserIDAndIdempotencyKey(
	ctx context.Context,
	userID uuid.UUID,
	key string,
) (*order.Order, error) {
	db := database.DB(ctx, r.pool)
	var o order.Order
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
			return nil, apperror.ErrNotFound
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

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]order.Order, error) {
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

func (r *Repository) ListAdmin(ctx context.Context, params order.AdminListParams) ([]order.Order, int, error) {
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
	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, 0, fmt.Errorf("listing orders: %w", err)
	}

	return orders, total, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, fromStatus, toStatus order.Status) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2 AND status = $3 RETURNING id`,
		toStatus, id, fromStatus,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating order status: %w", err)
	}
	return nil
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

// Apply runs the guarded compare-and-set for a Transition: it moves the order to
// t.To only if its current status is one of t.From, returning apperror.ErrConflict
// if nothing matched.
func (r *Repository) Apply(ctx context.Context, id uuid.UUID, t order.Transition) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	// The stock flags ride along in the same CAS so they can never disagree with
	// the status. OR keeps them monotonic: once deducted/reversed, applying a
	// transition that does not set the flag leaves it unchanged.
	err := db.QueryRow(ctx,
		`UPDATE orders SET status = $1,
		        stock_deducted = stock_deducted OR $4,
		        stock_reversed = stock_reversed OR $5
		WHERE id = $2 AND status = ANY($3) RETURNING id`,
		t.To, id, t.From, t.SetsStockDeducted, t.SetsStockReversed,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("applying order transition: %w", err)
	}
	return nil
}

func (r *Repository) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]order.Item, error) {
	db := database.DB(ctx, r.pool)
	// Joined for o.currency only: an item's amounts are denominated in its order's
	// currency, and order_items has no column of its own to read it from. Both
	// tables belong to this module, so the join crosses no boundary.
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

func (r *Repository) GetExpiredOrders(ctx context.Context, limit int) ([]order.Order, error) {
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

func (r *Repository) GetStaleProcessingOrders(
	ctx context.Context,
	threshold time.Duration,
	limit int,
) ([]order.Order, error) {
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

// HasDeliveredOrder reports whether the given order is delivered, belongs to the
// user, and contains the product. Binding to the specific orderID (not just
// "some delivered order for this product") stops a review being filed against an
// order that isn't the reviewer's or never contained the product.
func (r *Repository) HasDeliveredOrder(ctx context.Context, p order.DeliveredPurchaseParams) (bool, error) {
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
