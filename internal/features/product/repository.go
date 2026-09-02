package product

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, p *domain.Product) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Product, error)
	ListPublished(ctx context.Context, params PublishedListParams) ([]domain.Product, string, bool, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Product, int, error)
	GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error)
	GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]domain.Image, error)
	CountPublishedByCategory(ctx context.Context, categoryID uuid.UUID) (int, error)
	Update(ctx context.Context, p *domain.Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	AddImage(ctx context.Context, img *domain.Image) error
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
}

type PublishedListParams struct {
	Cursor     string
	Limit      int
	CategoryID *uuid.UUID
	MinPrice   *int64
	MaxPrice   *int64
	Search     string
}

type AdminListParams struct {
	paging.OffsetPage

	Status     string
	CategoryID *uuid.UUID
	Search     string
}
