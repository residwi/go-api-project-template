package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/product"
)

func scanProduct(row pgx.CollectableRow) (product.Product, error) {
	var p product.Product
	err := row.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// scanProductIncludingDeleted additionally scans deleted_at, unlike scanProduct:
// every other query filters deleted_at IS NULL, so DeletedAt would always come
// back nil there; GetByIDsIncludingDeleted is the one caller that needs it, to
// report a withdrawn product's sellability honestly instead of via its
// (untouched) status column.
func scanProductIncludingDeleted(row pgx.CollectableRow) (product.Product, error) {
	var p product.Product
	err := row.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	return p, err
}

func scanImage(row pgx.CollectableRow) (product.Image, error) {
	var img product.Image
	err := row.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.SortOrder, &img.CreatedAt)
	return img, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, p *product.Product) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO products (category_id, name, slug, description, price, compare_at_price, currency, sku, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price, p.CompareAtPrice,
		p.Currency, p.SKU, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating product: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*product.Product, error) {
	db := database.DB(ctx, r.pool)
	var p product.Product
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by id: %w", err)
	}
	return &p, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*product.Product, error) {
	db := database.DB(ctx, r.pool)
	var p product.Product
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE slug = $1 AND deleted_at IS NULL`, slug,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by slug: %w", err)
	}
	return &p, nil
}

func (r *Repository) Update(ctx context.Context, p *product.Product) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE products SET category_id=$1, name=$2, slug=$3, description=$4, price=$5,
		        compare_at_price=$6, currency=$7, sku=$8, status=$9
		WHERE id = $10 AND deleted_at IS NULL`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price, p.CompareAtPrice,
		p.Currency, p.SKU, p.Status, p.ID,
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

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE products SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *Repository) ListPublished(ctx context.Context, params product.PublishedListParams) ([]product.Product, string, bool, error) {
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

	// Fetch one extra to determine hasMore
	limit := params.Limit + 1
	query := fmt.Sprintf(
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, limit)

	rows, err := db.Query(ctx, query, args...)
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

func (r *Repository) ListAdmin(ctx context.Context, params product.AdminListParams) ([]product.Product, int, error) {
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

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing products: %w", err)
	}
	products, err := pgx.CollectRows(rows, scanProduct)
	if err != nil {
		return nil, 0, fmt.Errorf("listing products: %w", err)
	}

	return products, total, nil
}

// GetByIDsIncludingDeleted returns products regardless of status or deleted_at,
// so a consumer holding a stale id (a cart line, a wishlist entry) can render
// what it has instead of dropping the row.
func (r *Repository) GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]product.Product, error) {
	if len(ids) == 0 {
		return []product.Product{}, nil
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

func (r *Repository) AddImage(ctx context.Context, img *product.Image) error {
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

func (r *Repository) GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]product.Image, error) {
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
