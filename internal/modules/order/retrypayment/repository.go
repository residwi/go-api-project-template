package retrypayment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
}
