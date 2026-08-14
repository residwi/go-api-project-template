package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/create"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ create.Repository = (*Repository)(nil)

func compareAtPriceAmount(p *domain.Product) *int64 {
	if p.CompareAtPrice == nil {
		return nil
	}
	return &p.CompareAtPrice.Amount
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, p *domain.Product) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO products (category_id, name, slug, description, price, compare_at_price, currency, sku, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price.Amount, compareAtPriceAmount(p),
		p.Price.Currency, p.SKU, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating product: %w", err)
	}
	return nil
}
