package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/modules/category/update"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ update.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	db := database.DB(ctx, r.pool)
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

func (r *Repository) Update(ctx context.Context, cat *domain.Category) error {
	db := database.DB(ctx, r.pool)
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

// AncestorDepthAndCycle reports the depth from parentID to the root (0 when
// parentID does not exist) and whether selfID is in that chain, which would make
// parentID a cycle.
// Bounded by the caller's own maxDepth so a corrupt chain cannot recurse forever
// and the two limits cannot drift; `<= maxDepth` reaches maxDepth+1, which is
// what the caller's `depth+1 > maxDepth` guard needs to fire.
func (r *Repository) AncestorDepthAndCycle(
	ctx context.Context,
	parentID, selfID uuid.UUID,
	maxDepth int,
) (int, bool, error) {
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
