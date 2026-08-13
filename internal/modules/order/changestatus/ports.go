package changestatus

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type TransitionPort interface {
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error
}
