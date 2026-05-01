package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error)
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error)
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
