package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/register"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ register.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

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
