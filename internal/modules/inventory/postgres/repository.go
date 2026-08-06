package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const stockValueCols = 2

// items is a map, so its keys are already unique -- the summing-by-product-id
// this used to do is dead once duplicates cannot reach it. Lock ordering
// belongs to lockLevels' `ORDER BY product_id`, not to ids.
func buildStockValues(items map[uuid.UUID]int) (string, []any, []uuid.UUID) {
	ids := make([]uuid.UUID, 0, len(items))
	for id := range items {
		ids = append(ids, id)
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
		args = append(args, id, items[id])
	}
	return strings.Join(placeholders, ","), args, ids
}

// Deterministic order, so overlapping batches cannot deadlock. Locking
// inventory_levels rather than products keeps a checkout from blocking an admin
// editing a name or price.
func lockLevels(ctx context.Context, db database.DBTX, ids []uuid.UUID) error {
	_, err := db.Exec(ctx,
		`SELECT 1 FROM inventory_levels WHERE product_id = ANY($1) ORDER BY product_id FOR UPDATE`, ids)
	if err != nil {
		return fmt.Errorf("locking inventory levels: %w", err)
	}
	return nil
}

// Total on hand is derived from the two stored columns, not stored itself.
func stockFrom(productID uuid.UUID, available, reserved int) *inventory.Stock {
	return &inventory.Stock{
		ProductID: productID,
		Quantity:  available + reserved,
		Reserved:  reserved,
		Available: available,
	}
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*inventory.Stock, error) {
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

func (r *Repository) Release(ctx context.Context, productID uuid.UUID, qty int) (*inventory.Stock, error) {
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

// ReserveBatch does not match a missing or short row, so RowsAffected below the
// input length means the whole batch is insufficient. The caller is in a
// transaction, so nothing stays reserved.
func (r *Repository) ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error {
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

// ReleaseBatch treats a partial match as an error: a skipped row keeps its
// reservation, and silent success would strand it.
func (r *Repository) ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error {
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

// DeductBatch moves only reserved_stock: available_stock was already reduced when
// the hold was taken.
func (r *Repository) DeductBatch(ctx context.Context, items map[uuid.UUID]int) error {
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

// RestockBatch leaves reserved_stock alone: DeductBatch already consumed it.
func (r *Repository) RestockBatch(ctx context.Context, items map[uuid.UUID]int) error {
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

func (r *Repository) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*inventory.Stock, error) {
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

func (r *Repository) Restock(ctx context.Context, productID uuid.UUID, qty int) (*inventory.Stock, error) {
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

func (r *Repository) GetStock(ctx context.Context, productID uuid.UUID) (*inventory.Stock, error) {
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

// AdjustStock sets available to the requested total minus what is reserved,
// because available is stored, and refuses a total below the reservations.
//
// The upsert is the only recovery for a product whose EnsureLevel never ran:
// GetStock, Restock and a bare UPDATE all 404 on the missing row. ON CONFLICT's
// WHERE clause still enforces the reserved-quantity guard -- no match means no
// RETURNING row, and the same error as before.
func (r *Repository) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*inventory.Stock, error) {
	db := database.DB(ctx, r.pool)
	var available, reserved int
	err := db.QueryRow(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		VALUES ($1, $2, 0)
		ON CONFLICT (product_id) DO UPDATE
		    SET available_stock = $2 - inventory_levels.reserved_stock
		WHERE inventory_levels.reserved_stock <= $2
		RETURNING available_stock, reserved_stock`,
		productID, newQuantity,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: cannot set stock below reserved quantity", apperror.ErrBadRequest)
		}
		return nil, fmt.Errorf("adjusting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

// EnsureLevel is idempotent, so a retry or a re-created product cannot clobber
// existing counts.
func (r *Repository) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 0, 0) ON CONFLICT (product_id) DO NOTHING`, productID)
	if err != nil {
		return fmt.Errorf("ensuring inventory level: %w", err)
	}
	return nil
}

// GetLevels leaves a missing id absent from the map: the caller decides what
// that means.
func (r *Repository) GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventory.Stock, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]inventory.Stock{}, nil
	}
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT product_id, available_stock, reserved_stock
		 FROM inventory_levels WHERE product_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting inventory levels: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]inventory.Stock, len(ids))
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
