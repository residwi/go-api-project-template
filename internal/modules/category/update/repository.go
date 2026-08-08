package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

// Repository is update's own storage. Its only implementation is
// update/postgres, constructed in category/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Category, error)
	Update(ctx context.Context, cat *domain.Category) error
	// AncestorDepthAndCycle backs the parent-chain validation create and update
	// both do; create declares its own copy on its own Repository, since a
	// slice may not import a sibling's port.
	AncestorDepthAndCycle(
		ctx context.Context,
		parentID, selfID uuid.UUID,
		maxDepth int,
	) (depth int, formsCycle bool, err error)
}
