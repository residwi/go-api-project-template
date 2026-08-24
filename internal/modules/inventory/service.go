package inventory

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Adjust(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error) {
	return s.repo.AdjustStock(ctx, productID, newQuantity)
}

func (s *Service) Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return s.repo.Restock(ctx, productID, qty)
}

func (s *Service) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	return s.repo.EnsureLevel(ctx, productID)
}

func (s *Service) GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error) {
	return s.repo.GetStock(ctx, productID)
}

func (s *Service) GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Availability, error) {
	levels, err := s.repo.GetLevels(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]Availability, len(levels))
	for id, st := range levels {
		out[id] = Availability{OnHand: st.Quantity, Available: st.Available}
	}
	return out, nil
}

func (s *Service) Reserve(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.Reserve(ctx, items)
}

func (s *Service) Deduct(ctx context.Context, items map[uuid.UUID]int) error {
	return s.repo.Deduct(ctx, items)
}

func (s *Service) Restore(ctx context.Context, items map[uuid.UUID]int, prior StockState) error {
	if prior == Deducted {
		return s.repo.RestockBatch(ctx, items)
	}
	return s.repo.ReleaseBatch(ctx, items)
}
