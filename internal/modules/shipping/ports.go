package shipping

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
)

type Orders interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
