package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ category.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func scanCategory(row pgx.CollectableRow) (domain.Category, error) {
	var c domain.Category
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) Create(ctx context.Context, cat *domain.Category) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(ctx,
		`INSERT INTO categories (name, slug, description, parent_id, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		cat.Name, cat.Slug, cat.Description, cat.ParentID, cat.SortOrder, cat.Active,
	).Scan(&cat.ID, &cat.CreatedAt, &cat.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating category: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	db := database.PrimaryDB(ctx, r.db)
	var c domain.Category
	err := db.QueryRow(ctx,
		`SELECT id, name, slug, description, parent_id, sort_order, active, created_at, updated_at
		FROM categories WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting category by id: %w", err)
	}
	return &c, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	db := database.PrimaryDB(ctx, r.db)
	var c domain.Category
	err := db.QueryRow(ctx,
		`SELECT id, name, slug, description, parent_id, sort_order, active, created_at, updated_at
		FROM categories WHERE slug = $1`, slug,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting category by slug: %w", err)
	}
	return &c, nil
}

func (r *Repository) List(ctx context.Context) ([]domain.Category, error) {
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) Update(ctx context.Context, cat *domain.Category) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx,
		`UPDATE categories SET name=$1, slug=$2, description=$3, parent_id=$4, sort_order=$5, active=$6
		WHERE id = $7`,
		cat.Name, cat.Slug, cat.Description, cat.ParentID, cat.SortOrder, cat.Active, cat.ID,
	)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx,
		`DELETE FROM categories WHERE id = $1`, id,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: category still has products or subcategories", apperror.ErrConflict)
		}
		return fmt.Errorf("deleting category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *Repository) AncestorDepthAndCycle(
	ctx context.Context,
	parentID, selfID uuid.UUID,
	maxDepth int,
) (int, bool, error) {
	db := database.PrimaryDB(ctx, r.db)
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
