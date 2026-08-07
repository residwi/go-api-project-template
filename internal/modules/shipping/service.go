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
