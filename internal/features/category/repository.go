package category

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/category/domain"
)

type Repository interface {
	Create(ctx context.Context, cat *domain.Category) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Category, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Category, error)
	List(ctx context.Context) ([]domain.Category, error)
	Update(ctx context.Context, cat *domain.Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	AncestorDepthAndCycle(
		ctx context.Context,
		parentID, selfID uuid.UUID,
		maxDepth int,
	) (depth int, formsCycle bool, err error)
}
