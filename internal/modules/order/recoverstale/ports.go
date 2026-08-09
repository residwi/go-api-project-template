package recoverstale

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// TransitionApplier reaches transition/ through this narrow port instead of
// importing it.
type TransitionApplier interface {
	Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error
}
