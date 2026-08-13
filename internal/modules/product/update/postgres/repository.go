package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/update"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ update.Repository = (*Repository)(nil)

type amountColumns struct {
	price          int64
	compareAtPrice *int64
	currency       string
}

func (a amountColumns) assignTo(p *domain.Product) {
	p.Price = money.New(a.price, a.currency)
	p.CompareAtPrice = nil
	if a.compareAtPrice != nil {
		compareAt := money.New(*a.compareAtPrice, a.currency)
		p.CompareAtPrice = &compareAt
	}
}

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

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	db := database.DB(ctx, r.pool)
	var p domain.Product
	var amt amountColumns
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &amt.price, &amt.compareAtPrice,
		&amt.currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by id: %w", err)
	}
	amt.assignTo(&p)
	return &p, nil
}

func (r *Repository) Update(ctx context.Context, p *domain.Product) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE products SET category_id=$1, name=$2, slug=$3, description=$4, price=$5,
		        compare_at_price=$6, currency=$7, sku=$8, status=$9
		WHERE id = $10 AND deleted_at IS NULL`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price.Amount, compareAtPriceAmount(p),
		p.Price.Currency, p.SKU, p.Status, p.ID,
	)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
