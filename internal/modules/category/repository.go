package category

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, cat *Category) error
	GetByID(ctx context.Context, id uuid.UUID) (*Category, error)
	GetBySlug(ctx context.Context, slug string) (*Category, error)
	Update(ctx context.Context, cat *Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]Category, error)
	AncestorDepthAndCycle(
		ctx context.Context,
		parentID, selfID uuid.UUID,
		maxDepth int,
	) (depth int, formsCycle bool, err error)
}
