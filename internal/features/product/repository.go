package product

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
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id uuid.UUID) (*Product, error)
	GetBySlug(ctx context.Context, slug string) (*Product, error)
	Update(ctx context.Context, p *Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListPublished(ctx context.Context, params PublishedListParams) ([]Product, string, bool, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]Product, int, error)
	AddImage(ctx context.Context, img *Image) error
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
	GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]Image, error)
}

type PublishedListParams struct {
	Cursor     string
	Limit      int
	CategoryID *uuid.UUID
	MinPrice   *int64
	MaxPrice   *int64
	Search     string
}

type AdminListParams struct {
	Page       int
	PageSize   int
	Status     string
	CategoryID *uuid.UUID
	Search     string
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, p *Product) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO products (category_id, name, slug, description, price, compare_at_price, currency, sku, stock_quantity, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, reserved_quantity, created_at, updated_at`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price, p.CompareAtPrice,
		p.Currency, p.SKU, p.StockQuantity, p.Status,
	).Scan(&p.ID, &p.ReservedQuantity, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating product: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	db := database.DB(ctx, r.pool)
	var p Product
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        stock_quantity, reserved_quantity, status, created_at, updated_at
		FROM products WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.StockQuantity, &p.ReservedQuantity, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by id: %w", err)
	}
	return &p, nil
}

func (r *PostgresRepository) GetBySlug(ctx context.Context, slug string) (*Product, error) {
	db := database.DB(ctx, r.pool)
	var p Product
	err := db.QueryRow(ctx,
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        stock_quantity, reserved_quantity, status, created_at, updated_at
		FROM products WHERE slug = $1 AND deleted_at IS NULL`, slug,
	).Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
		&p.Currency, &p.SKU, &p.StockQuantity, &p.ReservedQuantity, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting product by slug: %w", err)
	}
	return &p, nil
}

func (r *PostgresRepository) Update(ctx context.Context, p *Product) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE products SET category_id=$1, name=$2, slug=$3, description=$4, price=$5,
		        compare_at_price=$6, currency=$7, sku=$8, stock_quantity=$9, status=$10
		WHERE id = $11 AND deleted_at IS NULL`,
		p.CategoryID, p.Name, p.Slug, p.Description, p.Price, p.CompareAtPrice,
		p.Currency, p.SKU, p.StockQuantity, p.Status, p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("updating product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE products SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListPublished(ctx context.Context, params PublishedListParams) ([]Product, string, bool, error) {
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
		args = append(args, "%"+params.Search+"%")
		argIdx++
	}

	if params.Cursor != "" {
		createdAt, id, err := core.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("%w: %s", core.ErrBadRequest, err.Error())
		}
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, createdAt, id)
		argIdx += 2
	}

	// Fetch one extra to determine hasMore
	limit := params.Limit + 1
	query := fmt.Sprintf(
		`SELECT id, category_id, name, slug, description, price, compare_at_price, currency, sku,
		        stock_quantity, reserved_quantity, status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, limit)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("listing published products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
			&p.Currency, &p.SKU, &p.StockQuantity, &p.ReservedQuantity, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, "", false, fmt.Errorf("scanning product: %w", err)
		}
		products = append(products, p)
	}

	hasMore := len(products) > params.Limit
	if hasMore {
		products = products[:params.Limit]
	}

	var nextCursor string
	if hasMore && len(products) > 0 {
		last := products[len(products)-1]
		nextCursor = core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())
	}

	return products, nextCursor, hasMore, nil
}

func (r *PostgresRepository) ListAdmin(ctx context.Context, params AdminListParams) ([]Product, int, error) {
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
		args = append(args, "%"+params.Search+"%")
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
		        stock_quantity, reserved_quantity, status, created_at, updated_at
		FROM products WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Slug, &p.Description, &p.Price, &p.CompareAtPrice,
			&p.Currency, &p.SKU, &p.StockQuantity, &p.ReservedQuantity, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning product: %w", err)
		}
		products = append(products, p)
	}

	return products, total, nil
}

func (r *PostgresRepository) AddImage(ctx context.Context, img *Image) error {
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

func (r *PostgresRepository) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM product_images WHERE id = $1`, imageID,
	)
	if err != nil {
		return fmt.Errorf("deleting product image: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]Image, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, product_id, url, alt_text, sort_order, created_at
		FROM product_images WHERE product_id = $1 ORDER BY sort_order`, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting product images: %w", err)
	}
	defer rows.Close()

	var images []Image
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.SortOrder, &img.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning product image: %w", err)
		}
		images = append(images, img)
	}

	return images, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && contains(err.Error(), "duplicate key value violates unique constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
