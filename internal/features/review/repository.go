package review

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, rv *domain.Review) error
	HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error)
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]domain.Review, error)
	GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
