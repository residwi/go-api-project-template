package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// Repository is query's own storage. Its only implementation is
// query/postgres, constructed in user/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetStatusByID(ctx context.Context, id uuid.UUID) (active bool, tokenVersion int, err error)
	ListAdmin(ctx context.Context, params Params) ([]domain.User, int, error)
}

type Params struct {
	paging.OffsetPage

	Role   string
	Active *bool
	Search string
}
