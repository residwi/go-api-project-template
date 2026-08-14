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
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/credentials"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ credentials.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, user *domain.User) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, first_name, last_name, phone, role, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		user.Email, user.PasswordHash, user.FirstName, user.LastName,
		user.Phone, user.Role, user.Active,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
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

func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	db := database.DB(ctx, r.pool)
	var u domain.User
	err := db.QueryRow(
		ctx,
		`SELECT id, email, password_hash, first_name, last_name, phone, role, active, token_version, created_at, updated_at
		FROM users WHERE email = $1 AND deleted_at IS NULL`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FirstName, &u.LastName,
		&u.Phone, &u.Role, &u.Active, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting user by email: %w", err)
	}
	return &u, nil
}
