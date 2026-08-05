package review

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, review *Review) error
	GetByID(ctx context.Context, id uuid.UUID) (*Review, error)
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]Review, error)
	GetStats(ctx context.Context, productID uuid.UUID) (Stats, error)
	HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
