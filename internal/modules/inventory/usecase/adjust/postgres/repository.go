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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/adjust"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ adjust.Repository = (*Repository)(nil)

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

func (r *Repository) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error) {
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
