package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/remove"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ remove.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
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
