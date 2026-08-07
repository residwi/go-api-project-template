package deliver

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Command struct {
	repo   Repository
	tx     database.TxRunner
	orders OrderPort
}

func New(repo Repository, tx database.TxRunner, orders OrderPort) *Command {
	return &Command{repo: repo, tx: tx, orders: orders}
}

func (c *Command) Execute(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error) {
	// Read outside the transaction: the OrderID is needed to flip the order, and a
	// missing shipment should not open one.
	shipment, err := c.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	var delivered *domain.Shipment
	if err := c.tx.Run(ctx, func(txCtx context.Context) error {
		var markErr error
		delivered, markErr = c.repo.MarkDelivered(txCtx, shipmentID)
		if markErr != nil {
			return markErr
		}
		return c.orders.MarkDelivered(txCtx, shipment.OrderID)
	}); err != nil {
		return nil, err
	}

	return delivered, nil
}
