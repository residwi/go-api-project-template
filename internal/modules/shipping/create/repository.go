package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

// Repository is create's own storage. Its only implementation is
// create/postgres, constructed in shipping/module.go.
type Repository interface {
	Create(ctx context.Context, shipment *domain.Shipment) error
}
