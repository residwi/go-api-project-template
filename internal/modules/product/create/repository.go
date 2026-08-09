package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

// Repository is create's own storage. Its only implementation is
// create/postgres, constructed in product/module.go.
type Repository interface {
	Create(ctx context.Context, p *domain.Product) error
}
