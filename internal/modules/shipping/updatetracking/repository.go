package updatetracking

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

// Repository is updatetracking's own storage. Its only implementation is
// updatetracking/postgres, constructed in shipping/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	Update(ctx context.Context, shipment *domain.Shipment) error
}
