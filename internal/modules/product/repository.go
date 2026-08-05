package product

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// Repository is product's persistence port. The Postgres implementation lives
// in the postgres subpackage; this package never imports it.
type Repository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id uuid.UUID) (*Product, error)
	GetBySlug(ctx context.Context, slug string) (*Product, error)
	Update(ctx context.Context, p *Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListPublished(ctx context.Context, params PublishedListParams) ([]Product, string, bool, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]Product, int, error)
	GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]Product, error)
	AddImage(ctx context.Context, img *Image) error
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
	GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]Image, error)
	CountPublishedByCategory(ctx context.Context, categoryID uuid.UUID) (int, error)
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
