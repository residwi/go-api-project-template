package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
)

type Service struct {
	repo Repository
	pool *pgxpool.Pool
}

func NewService(repo Repository, pool *pgxpool.Pool) *Service {
	return &Service{repo: repo, pool: pool}
}

func (s *Service) Create(ctx context.Context, req CreateProductRequest) (*Product, error) {
	p := &Product{
		CategoryID:     req.CategoryID,
		Name:           req.Name,
		Slug:           core.Slugify(req.Name),
		Description:    req.Description,
		Price:          req.Price,
		CompareAtPrice: req.CompareAtPrice,
		Currency:       "USD",
		SKU:            req.SKU,
		Status:         StatusDraft,
	}

	if req.Currency != "" {
		p.Currency = req.Currency
	}
	if req.StockQuantity != nil {
		p.StockQuantity = *req.StockQuantity
	}
	if req.Status != "" {
		p.Status = req.Status
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*Product, error) {
	p, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	if p.Status != StatusPublished {
		return nil, core.ErrNotFound
	}

	images, err := s.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	return p, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	images, err := s.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	return p, nil
}

func (s *Service) ListPublished(ctx context.Context, params PublishedListParams) ([]Product, string, bool, error) {
	return s.repo.ListPublished(ctx, params)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]Product, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateProductRequest) (*Product, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.CategoryID != nil {
		p.CategoryID = req.CategoryID
	}
	if req.Name != nil {
		p.Name = *req.Name
		p.Slug = core.Slugify(p.Name)
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.Price != nil {
		p.Price = *req.Price
	}
	if req.CompareAtPrice != nil {
		p.CompareAtPrice = req.CompareAtPrice
	}
	if req.Currency != nil {
		p.Currency = *req.Currency
	}
	if req.SKU != nil {
		p.SKU = req.SKU
	}
	if req.StockQuantity != nil {
		p.StockQuantity = *req.StockQuantity
	}
	if req.Status != nil {
		p.Status = *req.Status
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) AddImage(ctx context.Context, productID uuid.UUID, req AddImageRequest) (*Image, error) {
	if _, err := s.repo.GetByID(ctx, productID); err != nil {
		return nil, err
	}

	img := &Image{
		ProductID: productID,
		URL:       req.URL,
		AltText:   req.AltText,
	}
	if req.SortOrder != nil {
		img.SortOrder = *req.SortOrder
	}

	if err := s.repo.AddImage(ctx, img); err != nil {
		return nil, err
	}

	return img, nil
}

func (s *Service) DeleteImage(ctx context.Context, productID, imageID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, productID); err != nil {
		return err
	}

	return s.repo.DeleteImage(ctx, imageID)
}

// AvailableQuantity returns stock - reserved for a given product.
func (s *Service) AvailableQuantity(ctx context.Context, id uuid.UUID) (int, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}
	avail := p.StockQuantity - p.ReservedQuantity
	if avail < 0 {
		return 0, fmt.Errorf("%w: negative available quantity", core.ErrInsufficientStock)
	}
	return avail, nil
}
