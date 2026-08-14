package manageimages

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	AddImage(ctx context.Context, img *domain.Image) error
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
}
