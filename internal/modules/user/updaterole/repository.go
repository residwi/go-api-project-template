package updaterole

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

// Repository is updaterole's own storage. Its only implementation is
// updaterole/postgres, constructed in user/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	CountAdmins(ctx context.Context) (int, error)
	IncrementTokenVersion(ctx context.Context, id uuid.UUID) error
}
