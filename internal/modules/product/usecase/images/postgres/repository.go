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
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/images"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ images.Repository = (*Repository)(nil)

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

func (r *Repository) AddImage(ctx context.Context, img *domain.Image) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO product_images (product_id, url, alt_text, sort_order)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		img.ProductID, img.URL, img.AltText, img.SortOrder,
	).Scan(&img.ID, &img.CreatedAt)
	if err != nil {
		return fmt.Errorf("adding product image: %w", err)
	}
	return nil
}

func (r *Repository) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM product_images WHERE id = $1`, imageID,
	)
	if err != nil {
		return fmt.Errorf("deleting product image: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
