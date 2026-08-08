package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// Repository is query's own storage. Its only implementation is
// query/postgres, constructed in review/module.go.
type Repository interface {
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]domain.Review, error)
	GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error)
}
