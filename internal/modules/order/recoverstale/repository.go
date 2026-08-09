package recoverstale

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is recoverstale's own storage. Its only implementation is
// recoverstale/postgres, constructed in order/module.go. It never writes
// status -- that goes through TransitionApplier -- so it only reads.
type Repository interface {
	GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]domain.Order, error)
}
