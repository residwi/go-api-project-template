package deliver

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type UseCase struct {
	repo      Repository
	tx        database.TxRunner
	deliverer OrderDeliverer
}

func New(repo Repository, tx database.TxRunner, deliverer OrderDeliverer) *UseCase {
	return &UseCase{repo: repo, tx: tx, deliverer: deliverer}
}

func (c *UseCase) Execute(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error) {
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
		return c.deliverer.MarkDelivered(txCtx, shipment.OrderID)
	}); err != nil {
		return nil, err
	}

	return delivered, nil
}
