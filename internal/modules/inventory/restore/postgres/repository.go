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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/restore"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ restore.Repository = (*Repository)(nil)

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
func stockFrom(productID uuid.UUID, available, reserved int) *domain.Stock {
	return &domain.Stock{
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

func (r *Repository) Release(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
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
