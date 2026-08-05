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

func (s *Service) ReserveBatch(ctx context.Context, items []StockChange) error {
	return s.repo.ReserveBatch(ctx, items)
}

func (s *Service) ReleaseBatch(ctx context.Context, items []StockChange) error {
	return s.repo.ReleaseBatch(ctx, items)
}

func (s *Service) DeductBatch(ctx context.Context, items []StockChange) error {
	return s.repo.DeductBatch(ctx, items)
}

func (s *Service) RestockBatch(ctx context.Context, items []StockChange) error {
	return s.repo.RestockBatch(ctx, items)
}

// Restore keeps the release-vs-restock choice here, so callers need not know that
// a reservation and a deduction unwind differently.
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

// EnsureLevel registers at zero stock: the initial quantity is set afterwards
// through inventory's own admin endpoint, not smuggled in on the product payload.
func (s *Service) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	return s.repo.EnsureLevel(ctx, productID)
}

func (s *Service) GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Stock, error) {
	return s.repo.GetLevels(ctx, ids)
}
