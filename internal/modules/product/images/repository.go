package images

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

// Repository is images' own storage. Its only implementation is
// images/postgres, constructed in product/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	AddImage(ctx context.Context, img *domain.Image) error
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
}
