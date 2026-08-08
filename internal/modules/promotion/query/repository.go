package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

// Repository is query's own storage. Its only implementation is
// query/postgres, constructed in promotion/module.go.
type Repository interface {
	ListAdmin(ctx context.Context, params Params) ([]domain.Promotion, int, error)
}
