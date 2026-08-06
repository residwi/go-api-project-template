package inventory

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
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

func (s *Service) ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.ReserveBatch(ctx, items)
}

func (s *Service) ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.ReleaseBatch(ctx, items)
}

func (s *Service) DeductBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.DeductBatch(ctx, items)
}

func (s *Service) RestockBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.RestockBatch(ctx, items)
}

// Restore keeps the release-vs-restock choice here, so callers need not know that
// a reservation and a deduction unwind differently.
func (s *Service) Restore(ctx context.Context, items map[uuid.UUID]int, prior contract.StockState) error {
	if prior == contract.Deducted {
		return s.repo.RestockBatch(ctx, items)
	}
	return s.repo.ReleaseBatch(ctx, items)
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

// GetAvailability answers for a whole page at once. A per-product lookup would
// turn every list endpoint into N+1 queries.
func (s *Service) GetAvailability(
	ctx context.Context,
	ids []uuid.UUID,
) (map[uuid.UUID]contract.Availability, error) {
	levels, err := s.repo.GetLevels(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]contract.Availability, len(levels))
	for id, st := range levels {
		out[id] = contract.Availability{OnHand: st.Quantity, Available: st.Available}
	}
	return out, nil
}
