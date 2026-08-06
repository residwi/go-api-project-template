package shipping

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
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

func (s *Service) CreateShipment(ctx context.Context, orderID uuid.UUID, p CreateParams) (*Shipment, error) {
	order, err := s.orders.GetInfo(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.Status != "paid" && order.Status != "processing" {
		return nil, fmt.Errorf("%w: order must be in paid or processing status", apperror.ErrBadRequest)
	}

	shipment := &Shipment{
		OrderID:        orderID,
		Carrier:        p.Carrier,
		TrackingNumber: p.TrackingNumber,
		Status:         StatusShipped,
	}

	// Create the shipment and flip the order to shipped atomically — a failed
	// order update rolls back the shipment instead of orphaning it.
	if err := s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, shipment); err != nil {
			return err
		}
		return s.updater.MarkShipped(txCtx, orderID)
	}); err != nil {
		return nil, err
	}

	return shipment, nil
}

func (s *Service) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error) {
	return s.repo.GetByOrderID(ctx, orderID)
}

func (s *Service) UpdateTracking(ctx context.Context, shipmentID uuid.UUID, p UpdateTrackingParams) (*Shipment, error) {
	shipment, err := s.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	if p.Carrier != "" {
		shipment.Carrier = p.Carrier
	}
	if p.TrackingNumber != "" {
		shipment.TrackingNumber = p.TrackingNumber
	}

	if err := s.repo.Update(ctx, shipment); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, shipmentID)
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
