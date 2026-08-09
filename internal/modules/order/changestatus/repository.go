package changestatus

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is changestatus's own storage. Its only implementation is
// changestatus/postgres, constructed in order/module.go. It only reads: the
// actual status write goes through TransitionPort.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
}
