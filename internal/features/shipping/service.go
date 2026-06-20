package shipping

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
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
	pool    *pgxpool.Pool
	orders  OrderProvider
	updater OrderUpdater
}

func NewService(repo Repository, pool *pgxpool.Pool, orders OrderProvider, updater OrderUpdater) *Service {
	return &Service{repo: repo, pool: pool, orders: orders, updater: updater}
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

	// Create the shipment and flip the order to shipped atomically — a failed
	// order update rolls back the shipment instead of orphaning it.
	if err := database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, shipment); err != nil {
			return err
		}
		return s.updater.UpdateStatus(txCtx, orderID, []string{"paid", "processing"}, "shipped")
	}); err != nil {
		return nil, err
	}

	return shipment, nil
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

	// Mark the shipment delivered and flip the order to delivered atomically — a
	// failed order update rolls back the shipment instead of diverging from it.
	if err := database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if err := s.repo.MarkDelivered(txCtx, shipmentID); err != nil {
			return err
		}
		return s.updater.UpdateStatus(txCtx, shipment.OrderID, []string{"shipped"}, "delivered")
	}); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, shipmentID)
}
