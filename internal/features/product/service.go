package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

type Service struct {
	repo Repository
	inv  InventoryReader
}

func NewService(repo Repository, inv InventoryReader) *Service {
	return &Service{repo: repo, inv: inv}
}

// enrich fills Availability for a page of products in one call to inventory.
// Products with no level row (never registered) read as zero rather than
// erroring, so a missing row cannot take down a listing.
func (s *Service) enrich(ctx context.Context, products []Product) error {
	if len(products) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(products))
	for i := range products {
		ids[i] = products[i].ID
	}
	levels, err := s.inv.GetAvailability(ctx, ids)
	if err != nil {
		return fmt.Errorf("reading availability: %w", err)
	}
	for i := range products {
		products[i].Availability = levels[products[i].ID]
	}
	return nil
}

func (s *Service) Create(ctx context.Context, req CreateProductRequest) (*Product, error) {
	p := &Product{
		CategoryID:     req.CategoryID,
		Name:           req.Name,
		Slug:           slug.MakeOrFallback(req.Name, "product-"+uuid.New().String()[:8]),
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
		return nil, apperror.ErrNotFound
	}

	images, err := s.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	one := []Product{*p}
	if err := s.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
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

	one := []Product{*p}
	if err := s.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (s *Service) ListPublished(ctx context.Context, params PublishedListParams) ([]Product, string, bool, error) {
	products, nextCursor, hasMore, err := s.repo.ListPublished(ctx, params)
	if err != nil {
		return nil, "", false, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, "", false, err
	}
	return products, nextCursor, hasMore, nil
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]Product, int, error) {
	products, total, err := s.repo.ListAdmin(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, 0, err
	}
	return products, total, nil
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
		p.Slug = slug.MakeOrFallback(p.Name, "product-"+p.ID.String()[:8])
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
	if req.Status != nil {
		p.Status = *req.Status
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
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

// AvailableQuantity returns the sellable quantity for a given product, read
// through the InventoryReader port now that product no longer stores stock.
func (s *Service) AvailableQuantity(ctx context.Context, id uuid.UUID) (int, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return 0, err
	}
	levels, err := s.inv.GetAvailability(ctx, []uuid.UUID{id})
	if err != nil {
		return 0, err
	}
	avail := levels[id].Available
	if avail < 0 {
		return 0, fmt.Errorf("%w: negative available quantity", apperror.ErrInsufficientStock)
	}
	return avail, nil
}
