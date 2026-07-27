package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository is user's persistence port. The Postgres implementation lives
// in the postgres subpackage; this package never imports it.
type Repository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetStatusByID(ctx context.Context, id uuid.UUID) (active bool, tokenVersion int, err error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, params ListParams) ([]User, int, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	CountAdmins(ctx context.Context) (int, error)
	IncrementTokenVersion(ctx context.Context, id uuid.UUID) error
}

type ListParams struct {
	Page     int
	PageSize int
	Role     string
	Active   *bool
	Search   string
}
