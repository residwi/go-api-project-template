package deliver

import (
	"context"

	"github.com/google/uuid"
)

type OrderPort interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
