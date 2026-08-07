package create

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Params struct {
	Carrier        string
	TrackingNumber string
}

type Command struct {
	repo   Repository
	tx     database.TxRunner
	orders OrderPort
}

func New(repo Repository, tx database.TxRunner, orders OrderPort) *Command {
	return &Command{repo: repo, tx: tx, orders: orders}
}

func (c *Command) Execute(ctx context.Context, orderID uuid.UUID, p Params) (*domain.Shipment, error) {
	order, err := c.orders.GetInfo(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if !domain.CanShipOrder(order.Status) {
		return nil, fmt.Errorf("%w: order must be in paid or processing status", apperror.ErrBadRequest)
	}

	shipment := &domain.Shipment{
		OrderID:        orderID,
		Carrier:        p.Carrier,
		TrackingNumber: p.TrackingNumber,
		Status:         domain.StatusShipped,
	}

	// One transaction: a failed order flip rolls the shipment back instead of
	// orphaning it. Order's repository picks the transaction up from ctx.
	if err := c.tx.Run(ctx, func(txCtx context.Context) error {
		if err := c.repo.Create(txCtx, shipment); err != nil {
			return err
		}
		return c.orders.MarkShipped(txCtx, orderID)
	}); err != nil {
		return nil, err
	}

	return shipment, nil
}
