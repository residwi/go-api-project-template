package deliver

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

// Repository is deliver's own storage. Its only implementation is
// deliver/postgres, constructed in shipping/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
}
