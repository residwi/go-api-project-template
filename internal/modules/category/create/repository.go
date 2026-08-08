package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

// Repository is create's own storage. Its only implementation is
// create/postgres, constructed in category/module.go.
type Repository interface {
	Create(ctx context.Context, cat *domain.Category) error
	// AncestorDepthAndCycle backs the parent-chain validation create and update
	// both do; update declares its own copy on its own Repository, since a slice
	// may not import a sibling's port.
	AncestorDepthAndCycle(
		ctx context.Context,
		parentID, selfID uuid.UUID,
		maxDepth int,
	) (depth int, formsCycle bool, err error)
}
