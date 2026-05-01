package inventory

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	repo Repository
	pool *pgxpool.Pool
}

func NewService(repo Repository, pool *pgxpool.Pool) *Service {
	return &Service{repo: repo, pool: pool}
}

func (s *Service) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Reserve(ctx, productID, qty)
}

func (s *Service) Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error) {
	return s.repo.Release(ctx, productID, qty)
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
