package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markallread"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ markallread.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false`, userID,
	)
	if err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}
	return nil
}
