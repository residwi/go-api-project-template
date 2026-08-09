package updateprofile

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

// Repository is updateprofile's own storage. Its only implementation is
// updateprofile/postgres, constructed in user/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
}
