package shipping

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
)

type Repository interface {
	Create(ctx context.Context, shipment *domain.Shipment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	Update(ctx context.Context, shipment *domain.Shipment) error
}
