package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Repository interface {
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error)
}
