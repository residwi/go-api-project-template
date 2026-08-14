package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func scanCategory(row pgx.CollectableRow) (domain.Category, error) {
	var c domain.Category
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ParentID,
		&c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) List(ctx context.Context) ([]domain.Category, error) {
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

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	db := database.DB(ctx, r.pool)
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
