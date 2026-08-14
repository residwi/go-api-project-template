package deliver

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
}
