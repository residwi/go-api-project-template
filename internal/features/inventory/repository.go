package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
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
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Stock, error)
}

// stockValueCols is the number of columns per (product_id, qty) VALUES tuple.
const stockValueCols = 2

// buildStockValues aggregates items by product_id (summing quantities) and
// renders the VALUES placeholder list and flat args for a batched stock UPDATE
// joined against (product_id, qty) tuples. Aggregation is required for
// correctness: a duplicate product_id would otherwise join the product row to
// two VALUES tuples and apply only one of the quantities. ids holds the distinct
// product ids in first-seen order; len(ids) is the number of distinct product
// rows updated. Lock ordering is owned by lockLevels' SQL `ORDER BY product_id`,
// not the order of this slice (which only feeds the VALUES join, where order is moot).
func buildStockValues(items []StockChange) (string, []any, []uuid.UUID) {
	sums := make(map[uuid.UUID]int, len(items))
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if _, seen := sums[it.ProductID]; !seen {
			ids = append(ids, it.ProductID)
		}
		sums[it.ProductID] += it.Quantity
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)*stockValueCols)
	param := 1
	for i, id := range ids {
		idCol, qtyCol := param, param+1
		param += stockValueCols
		if i == 0 {
			// Cast the first row so Postgres infers the VALUES column types.
			placeholders[i] = fmt.Sprintf("($%d::uuid, $%d::int)", idCol, qtyCol)
		} else {
			placeholders[i] = fmt.Sprintf("($%d, $%d)", idCol, qtyCol)
		}
		args = append(args, id, sums[id])
	}
	return strings.Join(placeholders, ","), args, ids
}

// lockLevels takes row locks in a deterministic order so concurrent batches
// covering overlapping product rows cannot deadlock. Locking inventory_levels
// instead of the product table also means a checkout no longer blocks an admin
// editing a product's name or price.
func lockLevels(ctx context.Context, db database.DBTX, ids []uuid.UUID) error {
	_, err := db.Exec(ctx,
		`SELECT 1 FROM inventory_levels WHERE product_id = ANY($1) ORDER BY product_id FOR UPDATE`, ids)
	if err != nil {
		return fmt.Errorf("locking inventory levels: %w", err)
	}
	return nil
}

// stockFrom assembles a Stock from the two stored columns. Total on hand is
// derived as their sum rather than stored.
func stockFrom(productID uuid.UUID, available, reserved int) *Stock {
	return &Stock{
		ProductID: productID,
		Quantity:  available + reserved,
		Reserved:  reserved,
		Available: available,
	}
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels
		SET available_stock = available_stock - $1, reserved_stock = reserved_stock + $1
		WHERE product_id = $2 AND available_stock >= $1
		RETURNING available_stock, reserved_stock`,
		qty, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrInsufficientStock
		}
		return nil, fmt.Errorf("reserving stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *PostgresRepository) Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels
		SET available_stock = available_stock + $1, reserved_stock = reserved_stock - $1
		WHERE product_id = $2 AND reserved_stock >= $1
		RETURNING available_stock, reserved_stock`,
		qty, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot release more than reserved", apperror.ErrBadRequest)
		}
		return nil, fmt.Errorf("releasing stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

// ReserveBatch reserves stock for many product rows in a single UPDATE. If any row
// is missing or has insufficient available stock it won't match, so a
// RowsAffected count below the input length means at least one reservation
// failed and the whole batch is reported as insufficient stock (the caller runs
// this inside a transaction, so nothing is reserved).
func (r *PostgresRepository) ReserveBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args, ids := buildStockValues(items)
	if err := lockLevels(ctx, db, ids); err != nil {
		return err
	}
	tag, err := db.Exec(ctx,
		`UPDATE inventory_levels AS i
		SET available_stock = available_stock - v.qty,
		    reserved_stock  = reserved_stock  + v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE i.product_id = v.product_id AND i.available_stock >= v.qty`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("reserving stock batch: %w", err)
	}
	if int(tag.RowsAffected()) != len(ids) {
		return apperror.ErrInsufficientStock
	}
	return nil
}

// ReleaseBatch releases reserved stock for many product rows. A partial match is an
// error: a skipped row means the reservation stayed, and silent success would
// strand it.
func (r *PostgresRepository) ReleaseBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args, ids := buildStockValues(items)
	if err := lockLevels(ctx, db, ids); err != nil {
		return err
	}
	tag, err := db.Exec(ctx,
		`UPDATE inventory_levels AS i
		SET available_stock = available_stock + v.qty,
		    reserved_stock  = reserved_stock  - v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE i.product_id = v.product_id AND i.reserved_stock >= v.qty`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("releasing stock batch: %w", err)
	}
	if int(tag.RowsAffected()) != len(ids) {
		return fmt.Errorf("%w: cannot release more than reserved", apperror.ErrBadRequest)
	}
	return nil
}

// DeductBatch converts a reservation into a sale. With stored-available this
// only decrements reserved_stock: the goods have left, and available_stock was
// already reduced when the hold was taken.
func (r *PostgresRepository) DeductBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args, ids := buildStockValues(items)
	if err := lockLevels(ctx, db, ids); err != nil {
		return err
	}
	tag, err := db.Exec(ctx,
		`UPDATE inventory_levels AS i
		SET reserved_stock = reserved_stock - v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE i.product_id = v.product_id AND i.reserved_stock >= v.qty`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("deducting stock batch: %w", err)
	}
	if int(tag.RowsAffected()) != len(ids) {
		return fmt.Errorf("%w: cannot deduct more than reserved", apperror.ErrBadRequest)
	}
	return nil
}

// RestockBatch returns sold goods to the shelf. It does not touch
// reserved_stock: the reservation was already consumed by DeductBatch.
func (r *PostgresRepository) RestockBatch(ctx context.Context, items []StockChange) error {
	if len(items) == 0 {
		return nil
	}
	db := database.DB(ctx, r.pool)
	values, args, ids := buildStockValues(items)
	if err := lockLevels(ctx, db, ids); err != nil {
		return err
	}
	_, err := db.Exec(ctx,
		`UPDATE inventory_levels AS i
		SET available_stock = available_stock + v.qty
		FROM (VALUES `+values+`) AS v(product_id, qty)
		WHERE i.product_id = v.product_id`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("restocking batch: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels SET reserved_stock = reserved_stock - $1
		WHERE product_id = $2 AND reserved_stock >= $1
		RETURNING available_stock, reserved_stock`,
		qty, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot deduct stock", apperror.ErrBadRequest)
		}
		return nil, fmt.Errorf("deducting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *PostgresRepository) Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels SET available_stock = available_stock + $1
		WHERE product_id = $2
		RETURNING available_stock, reserved_stock`,
		qty, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("restocking: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *PostgresRepository) GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

// AdjustStock sets total on hand. Because available is stored, the new available
// is the requested total minus whatever is currently reserved -- and a total
// below the outstanding reservations is refused.
func (r *PostgresRepository) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels SET available_stock = $1 - reserved_stock
		WHERE product_id = $2 AND reserved_stock <= $1
		RETURNING available_stock, reserved_stock`,
		newQuantity, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot set stock below reserved quantity", apperror.ErrBadRequest)
		}
		return nil, fmt.Errorf("adjusting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

// EnsureLevel creates a zeroed level row for a product. Idempotent, so a retry
// or a re-created product cannot clobber existing counts.
func (r *PostgresRepository) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 0, 0) ON CONFLICT (product_id) DO NOTHING`, productID)
	if err != nil {
		return fmt.Errorf("ensuring inventory level: %w", err)
	}
	return nil
}

// GetLevels reads many products' levels in one query. Missing product ids are
// simply absent from the map; the caller decides what a missing level means.
func (r *PostgresRepository) GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Stock, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Stock{}, nil
	}
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT product_id, available_stock, reserved_stock
		 FROM inventory_levels WHERE product_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting inventory levels: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]Stock, len(ids))
	for rows.Next() {
		var id uuid.UUID
		var available, reserved int
		if err := rows.Scan(&id, &available, &reserved); err != nil {
			return nil, fmt.Errorf("scanning inventory level: %w", err)
		}
		out[id] = *stockFrom(id, available, reserved)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inventory levels: %w", err)
	}
	return out, nil
}
