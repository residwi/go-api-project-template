package wishlist

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Add(ctx context.Context, userID, productID uuid.UUID) error {
	wishlistID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.AddItem(ctx, wishlistID, productID)
}

func (s *Service) Remove(ctx context.Context, userID, productID uuid.UUID) error {
	return s.repo.RemoveItem(ctx, userID, productID)
}

func (s *Service) List(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Item, error) {
	return s.repo.ListItemsForUser(ctx, userID, cursor)
}
