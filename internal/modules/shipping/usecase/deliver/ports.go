package deliver

import (
	"context"

	"github.com/google/uuid"
)

type OrderDeliverer interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
