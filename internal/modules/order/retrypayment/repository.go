package retrypayment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is retrypayment's own storage. Its only implementation is
// retrypayment/postgres, constructed in order/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
}
