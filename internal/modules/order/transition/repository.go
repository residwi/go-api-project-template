package transition

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type Repository interface {
	Apply(ctx context.Context, id uuid.UUID, t domain.Transition) error
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error
}
