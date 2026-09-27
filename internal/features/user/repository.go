package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.User, int, error)
	UpdateProfile(ctx context.Context, user *domain.User) error
	UpdateGuarded(ctx context.Context, user *domain.User) (bool, error)
	Delete(ctx context.Context, id uuid.UUID) (bool, error)
	CountAdmins(ctx context.Context) (int, error)
	IncrementTokenVersion(ctx context.Context, id uuid.UUID) error
}

type AdminListParams struct {
	paging.OffsetPage

	Role   string
	Active *bool
	Search string
}
