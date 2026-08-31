package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var _ inventory.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error) {
	db := database.PrimaryDB(ctx, r.db)
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
			return nil, fmt.Errorf("%w: cannot set stock below reserved quantity", errs.ErrBadRequest)
		}
		return nil, fmt.Errorf("adjusting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *Repository) Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	db := database.PrimaryDB(ctx, r.db)
	var available, reserved int
	err := db.QueryRow(ctx,
		`UPDATE inventory_levels SET available_stock = available_stock + $1
		WHERE product_id = $2
		RETURNING available_stock, reserved_stock`,
		qty, productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("restocking: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *Repository) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	_, err := db.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 0, 0) ON CONFLICT (product_id) DO NOTHING`, productID)
	if err != nil {
		return fmt.Errorf("ensuring inventory level: %w", err)
	}
	return nil
}

func (r *Repository) GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error) {
	db := database.PrimaryDB(ctx, r.db)
	var available, reserved int
	err := db.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID,
	).Scan(&available, &reserved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("getting stock: %w", err)
	}
	return stockFrom(productID, available, reserved), nil
}

func (r *Repository) GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Stock, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]domain.Stock{}, nil
	}
	db := database.PrimaryDB(ctx, r.db)
	rows, err := db.Query(ctx,
		`SELECT product_id, available_stock, reserved_stock
		 FROM inventory_levels WHERE product_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting inventory levels: %w", err)
	}
	levels, err := pgx.CollectRows(rows, scanLevel)
	if err != nil {
		return nil, fmt.Errorf("scanning inventory levels: %w", err)
	}

	out := make(map[uuid.UUID]domain.Stock, len(levels))
	for _, level := range levels {
		out[level.ProductID] = level
	}
	return out, nil
}

func (r *Repository) Reserve(ctx context.Context, items map[uuid.UUID]int) error {
	if len(items) == 0 {
		return nil
	}
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) Deduct(ctx context.Context, items map[uuid.UUID]int) error {
	if len(items) == 0 {
		return nil
	}
	db := database.PrimaryDB(ctx, r.db)
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
		return fmt.Errorf("%w: cannot deduct more than reserved", errs.ErrBadRequest)
	}
	return nil
}

func (r *Repository) ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error {
	if len(items) == 0 {
		return nil
	}
	db := database.PrimaryDB(ctx, r.db)
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
		return fmt.Errorf("%w: cannot release more than reserved", errs.ErrBadRequest)
	}
	return nil
}

func (r *Repository) RestockBatch(ctx context.Context, items map[uuid.UUID]int) error {
	if len(items) == 0 {
		return nil
	}
	db := database.PrimaryDB(ctx, r.db)
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

const stockValueCols = 2

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
			placeholders[i] = fmt.Sprintf("($%d::uuid, $%d::int)", idCol, qtyCol)
		} else {
			placeholders[i] = fmt.Sprintf("($%d, $%d)", idCol, qtyCol)
		}
		args = append(args, id, items[id])
	}
	return strings.Join(placeholders, ","), args, ids
}

func lockLevels(ctx context.Context, db database.DBTX, ids []uuid.UUID) error {
	_, err := db.Exec(ctx,
		`SELECT 1 FROM inventory_levels WHERE product_id = ANY($1) ORDER BY product_id FOR UPDATE`, ids)
	if err != nil {
		return fmt.Errorf("locking inventory levels: %w", err)
	}
	return nil
}

func stockFrom(productID uuid.UUID, available, reserved int) *domain.Stock {
	return &domain.Stock{
		ProductID: productID,
		Quantity:  available + reserved,
		Reserved:  reserved,
		Available: available,
	}
}

func scanLevel(row pgx.CollectableRow) (domain.Stock, error) {
	var id uuid.UUID
	var available, reserved int
	if err := row.Scan(&id, &available, &reserved); err != nil {
		return domain.Stock{}, err
	}
	return *stockFrom(id, available, reserved), nil
}
