package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

// Repository is update's own storage. Its only implementation is
// update/postgres, constructed in product/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	Update(ctx context.Context, p *domain.Product) error
}
