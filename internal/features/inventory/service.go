package inventory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Reserve(ctx, productID, qty)
}

func (s *Service) Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Release(ctx, productID, qty)
}

// ReserveBatch reserves stock for many products in one query (used at checkout).
func (s *Service) ReserveBatch(ctx context.Context, items []StockChange) error {
	return s.repo.ReserveBatch(ctx, items)
}

// ReleaseBatch releases reserved stock for many products in one query.
func (s *Service) ReleaseBatch(ctx context.Context, items []StockChange) error {
	return s.repo.ReleaseBatch(ctx, items)
}

// DeductBatch deducts stock for many products in one query (used on payment success).
func (s *Service) DeductBatch(ctx context.Context, items []StockChange) error {
	return s.repo.DeductBatch(ctx, items)
}

// RestockBatch restocks many products in one query (used on refund).
func (s *Service) RestockBatch(ctx context.Context, items []StockChange) error {
	return s.repo.RestockBatch(ctx, items)
}

// Restore reverses an order's inventory effect: reserved stock is released,
// deducted stock is restocked. Inventory owns this choice so callers don't have
// to know that a reservation and a deduction unwind differently.
func (s *Service) Restore(ctx context.Context, items []StockChange, prior StockState) error {
	switch prior {
	case Deducted:
		return s.repo.RestockBatch(ctx, items)
	case Reserved:
		return s.repo.ReleaseBatch(ctx, items)
	default:
		return fmt.Errorf("unknown stock state: %d", prior)
	}
}

func (s *Service) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Deduct(ctx, productID, qty)
}

func (s *Service) Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Restock(ctx, productID, qty)
}

func (s *Service) GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error) {
	return s.repo.GetStock(ctx, productID)
}

func (s *Service) AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error) {
	return s.repo.AdjustStock(ctx, productID, newQuantity)
}
