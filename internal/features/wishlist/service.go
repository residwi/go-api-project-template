package wishlist

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
)

type Service struct {
	repo Repository
	pool *pgxpool.Pool
}

func NewService(repo Repository, pool *pgxpool.Pool) *Service {
	return &Service{repo: repo, pool: pool}
}

func (s *Service) GetWishlist(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Item, error) {
	return s.repo.GetItems(ctx, userID, cursor)
}

func (s *Service) AddItem(ctx context.Context, userID, productID uuid.UUID) error {
	wishlistID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.AddItem(ctx, wishlistID, productID)
}

func (s *Service) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
	wishlistID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.RemoveItem(ctx, wishlistID, productID)
}
