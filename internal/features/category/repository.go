package category

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

func scanCategory(row pgx.CollectableRow) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

type Repository interface {
	Create(ctx context.Context, cat *Category) error
	GetByID(ctx context.Context, id uuid.UUID) (*Category, error)
	GetBySlug(ctx context.Context, slug string) (*Category, error)
	Update(ctx context.Context, cat *Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]Category, error)
	CountPublishedProducts(ctx context.Context, categoryID uuid.UUID) (int, error)
	AncestorDepthAndCycle(ctx context.Context, parentID, selfID uuid.UUID, maxDepth int) (depth int, formsCycle bool, err error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, cat *Category) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO categories (name, slug, description, parent_id, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		cat.Name, cat.Slug, cat.Description, cat.ParentID, cat.SortOrder, cat.Active,
	).Scan(&cat.ID, &cat.CreatedAt, &cat.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("creating category: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Category, error) {
	db := database.DB(ctx, r.pool)
	var c Category
	err := db.QueryRow(ctx,
		`SELECT id, name, slug, description, parent_id, sort_order, active, created_at, updated_at
		FROM categories WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting category by id: %w", err)
	}
	return &c, nil
}

func (r *PostgresRepository) GetBySlug(ctx context.Context, slug string) (*Category, error) {
	db := database.DB(ctx, r.pool)
	var c Category
	err := db.QueryRow(ctx,
		`SELECT id, name, slug, description, parent_id, sort_order, active, created_at, updated_at
		FROM categories WHERE slug = $1`, slug,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting category by slug: %w", err)
	}
	return &c, nil
}

func (r *PostgresRepository) Update(ctx context.Context, cat *Category) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE categories SET name=$1, slug=$2, description=$3, parent_id=$4, sort_order=$5, active=$6
		WHERE id = $7`,
		cat.Name, cat.Slug, cat.Description, cat.ParentID, cat.SortOrder, cat.Active, cat.ID,
	)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("updating category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM categories WHERE id = $1`, id,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: category still has products or subcategories", core.ErrConflict)
		}
		return fmt.Errorf("deleting category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Category, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, name, slug, description, parent_id, sort_order, active, created_at, updated_at
		FROM categories ORDER BY sort_order, name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}
	categories, err := pgx.CollectRows(rows, scanCategory)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}

	return categories, nil
}

func (r *PostgresRepository) CountPublishedProducts(ctx context.Context, categoryID uuid.UUID) (int, error) {
	db := database.DB(ctx, r.pool)
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM products WHERE category_id = $1 AND status = 'published' AND deleted_at IS NULL`,
		categoryID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting published products: %w", err)
	}
	return count, nil
}

// AncestorDepthAndCycle walks the parent chain upward from parentID via a
// recursive CTE and reports the depth from parentID to the root (0 if parentID
// does not exist) and whether selfID appears in that chain — selfID being an
// ancestor of parentID means setting parentID as selfID's parent forms a cycle.
// The walk is bounded by maxDepth (the caller's depth limit) so a corrupt chain
// cannot recurse without limit; the bound is derived from the limit rather than
// hardcoded so the two cannot drift. `a.depth <= maxDepth` lets the walk reach
// maxDepth+1, enough for the caller's `depth+1 > maxDepth` guard to fire.
func (r *PostgresRepository) AncestorDepthAndCycle(ctx context.Context, parentID, selfID uuid.UUID, maxDepth int) (int, bool, error) {
	db := database.DB(ctx, r.pool)
	var (
		depth      int
		formsCycle bool
	)
	err := db.QueryRow(ctx,
		`WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, 1 AS depth
			FROM categories WHERE id = $1
			UNION ALL
			SELECT c.id, c.parent_id, a.depth + 1
			FROM categories c
			JOIN ancestors a ON a.parent_id = c.id
			WHERE a.depth <= $3
		)
		SELECT COALESCE(MAX(depth), 0), COUNT(*) FILTER (WHERE id = $2) > 0 FROM ancestors`,
		parentID, selfID, maxDepth,
	).Scan(&depth, &formsCycle)
	if err != nil {
		return 0, false, fmt.Errorf("walking category ancestors: %w", err)
	}
	return depth, formsCycle, nil
}
