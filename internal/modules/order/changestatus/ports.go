package changestatus

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// TransitionPort reaches transition/'s dynamic from/to write through this
// narrow port instead of importing it, so the status column still has exactly
// one writer.
type TransitionPort interface {
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error
}
