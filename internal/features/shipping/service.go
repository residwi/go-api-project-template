package shipping

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

type OrderProvider interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderInfo, error)
}

type OrderInfo struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Status string
}

type OrderUpdater interface {
	UpdateStatus(ctx context.Context, orderID uuid.UUID, fromStatuses []string, toStatus string) error
}

type Service struct {
	repo    Repository
	orders  OrderProvider
	updater OrderUpdater
}

func NewService(repo Repository, orders OrderProvider, updater OrderUpdater) *Service {
	return &Service{repo: repo, orders: orders, updater: updater}
}

func (s *Service) CreateShipment(ctx context.Context, orderID uuid.UUID, req CreateShipmentRequest) (*Shipment, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.Status != "paid" && order.Status != "processing" {
		return nil, fmt.Errorf("%w: order must be in paid or processing status", core.ErrBadRequest)
	}

	shipment := &Shipment{
		OrderID:        orderID,
		Carrier:        req.Carrier,
		TrackingNumber: req.TrackingNumber,
		Status:         StatusShipped,
	}

	if err := s.repo.Create(ctx, shipment); err != nil {
		return nil, err
	}

	if err := s.repo.MarkShipped(ctx, shipment.ID); err != nil {
		return nil, err
	}

	if err := s.updater.UpdateStatus(ctx, orderID, []string{"paid", "processing"}, "shipped"); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, shipment.ID)
}

func (s *Service) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error) {
	return s.repo.GetByOrderID(ctx, orderID)
}

func (s *Service) UpdateTracking(ctx context.Context, shipmentID uuid.UUID, req UpdateTrackingRequest) (*Shipment, error) {
	shipment, err := s.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	if req.Carrier != "" {
		shipment.Carrier = req.Carrier
	}
	if req.TrackingNumber != "" {
		shipment.TrackingNumber = req.TrackingNumber
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

	if err := s.repo.MarkDelivered(ctx, shipmentID); err != nil {
		return nil, err
	}

	if err := s.updater.UpdateStatus(ctx, shipment.OrderID, []string{"shipped"}, "delivered"); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, shipmentID)
}
