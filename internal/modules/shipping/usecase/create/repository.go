package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Repository interface {
	Create(ctx context.Context, shipment *domain.Shipment) error
}
