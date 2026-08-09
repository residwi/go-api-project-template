package remove

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

// Repository is remove's own storage. Its only implementation is
// remove/postgres, constructed in user/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	CountAdmins(ctx context.Context) (int, error)
}
