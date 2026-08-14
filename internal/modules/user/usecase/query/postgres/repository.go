package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

func scanUser(row pgx.CollectableRow) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	db := database.DB(ctx, r.pool)
	var u domain.User
	err := db.QueryRow(
		ctx,
		`SELECT id, email, password_hash, first_name, last_name, phone, role, active, token_version, created_at, updated_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FirstName, &u.LastName,
		&u.Phone, &u.Role, &u.Active, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return &u, nil
}

func (r *Repository) GetStatusByID(ctx context.Context, id uuid.UUID) (bool, int, error) {
	db := database.DB(ctx, r.pool)
	var active bool
	var tokenVersion int
	err := db.QueryRow(ctx,
		`SELECT active, token_version FROM users WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&active, &tokenVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, apperror.ErrNotFound
		}
		return false, 0, fmt.Errorf("getting user status by id: %w", err)
	}
	return active, tokenVersion, nil
}

func (r *Repository) ListAdmin(ctx context.Context, params query.Params) ([]domain.User, int, error) {
	db := database.DB(ctx, r.pool)

	where := "deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if params.Role != "" {
		where += fmt.Sprintf(" AND role = $%d", argIdx)
		args = append(args, params.Role)
		argIdx++
	}
	if params.Active != nil {
		where += fmt.Sprintf(" AND active = $%d", argIdx)
		args = append(args, *params.Active)
		argIdx++
	}
	if params.Search != "" {
		where += fmt.Sprintf(
			" AND (first_name ILIKE $%d OR last_name ILIKE $%d OR email ILIKE $%d)",
			argIdx,
			argIdx,
			argIdx,
		)
		args = append(args, "%"+database.EscapeLike(params.Search)+"%")
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM users WHERE " + where
	if err := db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	listQuery := fmt.Sprintf(
		"SELECT id, email, first_name, last_name, phone, role, active, created_at, updated_at FROM users WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where,
		argIdx,
		argIdx+1,
	)
	args = append(args, params.Limit(), params.Offset())

	rows, err := db.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	users, err := pgx.CollectRows(rows, scanUser)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}

	return users, total, nil
}
