package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// StockChange is one product/quantity pair for a batched inventory operation.
type StockChange struct {
	ProductID uuid.UUID
	Quantity  int
}

type Repository interface {
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	ReserveBatch(ctx context.Context, items []StockChange) error
	ReleaseBatch(ctx context.Context, items []StockChange) error
	DeductBatch(ctx context.Context, items []StockChange) error
	RestockBatch(ctx context.Context, items []StockChange) error
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error)
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error)
}

// stockValueCols is the number of columns per (product_id, qty) VALUES tuple.
const stockValueCols = 2

// buildStockValues renders the VALUES placeholder list and flat args for a
// batched stock UPDATE joined against (product_id, qty) tuples.
func buildStockValues(items []StockChange) (string, []any) {
	placeholders := make([]string, len(items))
	args := make([]any, 0, len(items)*stockValueCols)
	param := 1
	for i, it := range items {
		idCol, qtyCol := param, param+1
		param += stockValueCols
		if i == 0 {
			// Cast the first row so Postgres infers the VALUES column types.
			placeholders[i] = fmt.Sprintf("($%d::uuid, $%d::int)", idCol, qtyCol)
		} else {
			placeholders[i] = fmt.Sprintf("($%d, $%d)", idCol, qtyCol)
		}
		args = append(args, it.ProductID, it.Quantity)
	}
	return strings.Join(placeholders, ","), args
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`UPDATE products SET reserved_quantity = reserved_quantity + $1
		WHERE id = $2 AND (stock_quantity - reserved_quantity) >= $1 AND deleted_at IS NULL
		RETURNING stock_quantity, reserved_quantity`,
		qty, productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrInsufficientStock
		}
		return nil, fmt.Errorf("reserving stock: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}

func (r *PostgresRepository) Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`UPDATE products SET reserved_quantity = reserved_quantity - $1
		WHERE id = $2 AND reserved_quantity >= $1
		RETURNING stock_quantity, reserved_quantity`,
		qty, productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot release more than reserved", core.ErrBadRequest)
		}
		return nil, fmt.Errorf("releasing stock: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}

// ReserveBatch reserves stock for many products in a single UPDATE. If any row
// is missing, deleted, or has insufficient available stock it won't match, so a
// RowsAffected count below the input length means at least one reservation
// failed and the whole batch is reported as insufficient stock (the caller runs
// this inside a transaction, so nothing is reserved).
func (r *PostgresRepository) ReserveBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args := buildStockValues(items)
	tag, err := db.Exec(ctx,
		`UPDATE products AS p SET reserved_quantity = reserved_quantity + v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE p.id = v.product_id AND (p.stock_quantity - p.reserved_quantity) >= v.qty AND p.deleted_at IS NULL`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("reserving stock batch: %w", err)
	}
	if int(tag.RowsAffected()) != len(items) {
		return core.ErrInsufficientStock
	}
	return nil
}

// ReleaseBatch releases reserved stock for many products in a single UPDATE.
// The reserved_quantity >= qty guard keeps reservations from going negative;
// releasing is best-effort, so a partial match is not treated as an error.
func (r *PostgresRepository) ReleaseBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args := buildStockValues(items)
	_, err := db.Exec(ctx,
		`UPDATE products AS p SET reserved_quantity = reserved_quantity - v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE p.id = v.product_id AND p.reserved_quantity >= v.qty`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("releasing stock batch: %w", err)
	}
	return nil
}

// DeductBatch converts reserved stock to sold (decrements both stock and
// reserved) for many products in one UPDATE. Every product must satisfy the
// guard, so a RowsAffected count below the input length is an error (the caller
// runs this in a transaction).
func (r *PostgresRepository) DeductBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args := buildStockValues(items)
	tag, err := db.Exec(ctx,
		`UPDATE products AS p SET stock_quantity = stock_quantity - v.qty, reserved_quantity = reserved_quantity - v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE p.id = v.product_id AND p.reserved_quantity >= v.qty AND p.stock_quantity >= v.qty`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("deducting stock batch: %w", err)
	}
	if int(tag.RowsAffected()) != len(items) {
		return fmt.Errorf("%w: cannot deduct stock", core.ErrBadRequest)
	}
	return nil
}

// RestockBatch adds quantities back to stock for many products in one UPDATE
// (used on refund/restock). Best-effort, like ReleaseBatch.
func (r *PostgresRepository) RestockBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args := buildStockValues(items)
	_, err := db.Exec(ctx,
		`UPDATE products AS p SET stock_quantity = stock_quantity + v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE p.id = v.product_id AND p.deleted_at IS NULL`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("restocking batch: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`UPDATE products SET stock_quantity = stock_quantity - $1, reserved_quantity = reserved_quantity - $1
		WHERE id = $2 AND reserved_quantity >= $1 AND stock_quantity >= $1
		RETURNING stock_quantity, reserved_quantity`,
		qty, productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot deduct stock", core.ErrBadRequest)
		}
		return nil, fmt.Errorf("deducting stock: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}

func (r *PostgresRepository) Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`UPDATE products SET stock_quantity = stock_quantity + $1
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING stock_quantity, reserved_quantity`,
		qty, productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("restocking: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}

func (r *PostgresRepository) GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`SELECT stock_quantity, reserved_quantity FROM products WHERE id = $1 AND deleted_at IS NULL`,
		productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting stock: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}

func (r *PostgresRepository) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var stockQty, reservedQty int
	err := db.QueryRow(ctx,
		`UPDATE products SET stock_quantity = $1
		WHERE id = $2 AND reserved_quantity <= $1 AND deleted_at IS NULL
		RETURNING stock_quantity, reserved_quantity`,
		newQuantity, productID,
	).Scan(&stockQty, &reservedQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot set stock below reserved quantity", core.ErrBadRequest)
		}
		return nil, fmt.Errorf("adjusting stock: %w", err)
	}
	return &Stock{
		ProductID: productID,
		Quantity:  stockQty,
		Reserved:  reservedQty,
		Available: stockQty - reservedQty,
	}, nil
}
