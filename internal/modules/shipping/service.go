package shipping

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Service struct {
	repo    Repository
	tx      database.TxRunner
	orders  OrderProvider
	updater OrderUpdater
}

func NewService(repo Repository, tx database.TxRunner, orders OrderProvider, updater OrderUpdater) *Service {
	return &Service{repo: repo, tx: tx, orders: orders, updater: updater}
}

func (s *Service) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error) {
	return s.repo.GetByOrderID(ctx, orderID)
}

func (s *Service) MarkDelivered(ctx context.Context, shipmentID uuid.UUID) (*Shipment, error) {
	shipment, err := s.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	// Atomic: a failed order update rolls the shipment back instead of diverging.
	var delivered *Shipment
	if err := s.tx.Run(ctx, func(txCtx context.Context) error {
		var markErr error
		delivered, markErr = s.repo.MarkDelivered(txCtx, shipmentID)
		if markErr != nil {
			return markErr
		}
		return s.updater.MarkDelivered(txCtx, shipment.OrderID)
	}); err != nil {
		return nil, err
	}

	return delivered, nil
}
