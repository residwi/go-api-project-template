package shipping

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

type Service struct {
	repo Repository
	tx   database.TxRunner

	orders Orders
}

func New(repo Repository, tx database.TxRunner, orders Orders) *Service {
	return &Service{repo: repo, tx: tx, orders: orders}
}

func (s *Service) Create(
	ctx context.Context,
	orderID uuid.UUID,
	carrier, trackingNumber string,
) (*domain.Shipment, error) {
	order, err := s.orders.Snapshot(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if !domain.CanShipOrder(order.Status) {
		return nil, fmt.Errorf("%w: order must be in paid or processing status", errs.ErrBadRequest)
	}

	shipment := &domain.Shipment{
		OrderID:        orderID,
		Carrier:        carrier,
		TrackingNumber: trackingNumber,
		Status:         domain.StatusShipped,
	}

	if err := s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, shipment); err != nil {
			return err
		}
		return s.orders.MarkShipped(txCtx, orderID)
	}); err != nil {
		return nil, err
	}

	return shipment, nil
}

func (s *Service) Deliver(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error) {
	shipment, err := s.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	var delivered *domain.Shipment
	if err := s.tx.Run(ctx, func(txCtx context.Context) error {
		var markErr error
		delivered, markErr = s.repo.MarkDelivered(txCtx, shipmentID)
		if markErr != nil {
			return markErr
		}
		return s.orders.MarkDelivered(txCtx, shipment.OrderID)
	}); err != nil {
		return nil, err
	}

	return delivered, nil
}

func (s *Service) UpdateTracking(
	ctx context.Context,
	shipmentID uuid.UUID,
	carrier, trackingNumber string,
) (*domain.Shipment, error) {
	shipment, err := s.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	if carrier != "" {
		shipment.Carrier = carrier
	}
	if trackingNumber != "" {
		shipment.TrackingNumber = trackingNumber
	}

	if err := s.repo.Update(ctx, shipment); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, shipmentID)
}

func (s *Service) GetForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Shipment, error) {
	order, err := s.orders.Snapshot(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.UserID != userID {
		return nil, errs.ErrNotFound
	}

	return s.repo.GetByOrderID(ctx, orderID)
}
