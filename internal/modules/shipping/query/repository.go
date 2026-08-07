package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

// Repository is query's own storage. Its only implementation is query/postgres,
// constructed in shipping/module.go.
type Repository interface {
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error)
}
