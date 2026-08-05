package shipping

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, shipment *Shipment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Shipment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error)
	Update(ctx context.Context, shipment *Shipment) error
	MarkShipped(ctx context.Context, id uuid.UUID) error
	MarkDelivered(ctx context.Context, id uuid.UUID) (*Shipment, error)
}
