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
	"github.com/residwi/go-api-project-template/internal/modules/product/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ query.Repository = (*Repository)(nil)

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

func scanProduct(row pgx.CollectableRow) (domain.Product, error) {
	var p domain.Product
	var amt amountColumns
	err := row.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &amt.price, &amt.compareAtPrice,
		&amt.currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	amt.assignTo(&p)
	return p, nil
}

func scanProductIncludingDeleted(row pgx.CollectableRow) (domain.Product, error) {
	var p domain.Product
	var amt amountColumns
	err := row.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &amt.price, &amt.compareAtPrice,
		&amt.currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return p, err
	}
	amt.assignTo(&p)
	return p, nil
}

func scanImage(row pgx.CollectableRow) (domain.Image, error) {
	var img domain.Image
	err := row.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.SortOrder, &img.CreatedAt)
	return img, err
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

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	db := database.DB(ctx, r.pool)
	var p domain.Product
	var amt amountColumns
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE slug = $1 AND deleted_at IS NULL`, slug,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &amt.price, &amt.compareAtPrice,
		&amt.currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by slug: %w", err)
	}
	amt.assignTo(&p)
	return &p, nil
}

func (r *Repository) ListPublished(
	ctx context.Context,
	params query.PublishedListParams,
) ([]domain.Product, string, bool, error) {
	db := database.DB(ctx, r.pool)

	where := "deleted_at IS NULL AND status = 'published'"
	args := []any{}
	argIdx := 1

	if params.CategoryID != nil {
		where += fmt.Sprintf(" AND category_id = $%d", argIdx)
		args = append(args, *params.CategoryID)
		argIdx++
	}
	if params.MinPrice != nil {
		where += fmt.Sprintf(" AND price >= $%d", argIdx)
		args = append(args, *params.MinPrice)
		argIdx++
	}
	if params.MaxPrice != nil {
		where += fmt.Sprintf(" AND price <= $%d", argIdx)
		args = append(args, *params.MaxPrice)
		argIdx++
	}
	if params.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+database.EscapeLike(params.Search)+"%")
		argIdx++
	}

	if params.Cursor != "" {
		var err error
		where, args, argIdx, err = database.KeysetCursor(where, args, argIdx, "created_at, id", params.Cursor)
		if err != nil {
			return nil, "", false, err
		}
	}

	limit := params.Limit + 1
	sqlQuery := fmt.Sprintf(
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, limit)

	rows, err := db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("listing published products: %w", err)
	}
	products, err := pgx.CollectRows(rows, scanProduct)
	if err != nil {
		return nil, "", false, fmt.Errorf("listing published products: %w", err)
	}

	hasMore := len(products) > params.Limit
	if hasMore {
		products = products[:params.Limit]
	}

	var nextCursor string
	if hasMore && len(products) > 0 {
		last := products[len(products)-1]
		nextCursor = paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())
	}

	return products, nextCursor, hasMore, nil
}

func (r *Repository) ListAdmin(ctx context.Context, params query.AdminListParams) ([]domain.Product, int, error) {
	db := database.DB(ctx, r.pool)

	where := "deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if params.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, params.Status)
		argIdx++
	}
	if params.CategoryID != nil {
		where += fmt.Sprintf(" AND category_id = $%d", argIdx)
		args = append(args, *params.CategoryID)
		argIdx++
	}
	if params.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d OR sku ILIKE $%d)", argIdx, argIdx, argIdx)
		args = append(args, "%"+database.EscapeLike(params.Search)+"%")
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM products WHERE " + where
	if err := db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting products: %w", err)
	}

	sqlQuery := fmt.Sprintf(
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.Limit(), params.Offset())

	rows, err := db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing products: %w", err)
	}
	products, err := pgx.CollectRows(rows, scanProduct)
	if err != nil {
		return nil, 0, fmt.Errorf("listing products: %w", err)
	}

	return products, total, nil
}

func (r *Repository) GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error) {
	if len(ids) == 0 {
		return []domain.Product{}, nil
	}
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price,
		        currency, sku, status, created_at, updated_at, deleted_at
		FROM products WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting products by ids: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanProductIncludingDeleted)
}

func (r *Repository) GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]domain.Image, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, product_id, url, alt_text, sort_order, created_at
		FROM product_images WHERE product_id = $1 ORDER BY sort_order`, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting product images: %w", err)
	}
	images, err := pgx.CollectRows(rows, scanImage)
	if err != nil {
		return nil, fmt.Errorf("getting product images: %w", err)
	}

	return images, nil
}

func (r *Repository) CountPublishedByCategory(ctx context.Context, categoryID uuid.UUID) (int, error) {
	db := database.DB(ctx, r.pool)
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM products
		 WHERE category_id = $1 AND status = 'published' AND deleted_at IS NULL`,
		categoryID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting published products: %w", err)
	}
	return count, nil
}
