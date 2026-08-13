package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

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

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error) {
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

func (r *Repository) GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Stock, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]domain.Stock{}, nil
	}
	db := database.DB(ctx, r.pool)
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
