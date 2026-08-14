package recoverstale

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type Repository interface {
	GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]domain.Order, error)
}
