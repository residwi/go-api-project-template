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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/restock"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ restock.Repository = (*Repository)(nil)

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

func (r *Repository) Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
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
