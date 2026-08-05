package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

// A business default, not a transport one: a seeder or CLI has no request to
// read a currency from.
const defaultCurrency = "USD"

// denominateLike restates an optional amount in price's currency, nil passing
// through. products stores one currency for both, so any other would be lost on
// the way to the database and re-read as the price's.
func denominateLike(amount *money.Money, price money.Money) *money.Money {
	if amount == nil {
		return nil
	}
	restated := money.New(amount.Amount, price.Currency)
	return &restated
}

type Service struct {
	repo Repository
	inv  InventoryReader
	reg  InventoryRegistrar
}

func NewService(repo Repository, inv InventoryReader, reg InventoryRegistrar) *Service {
	return &Service{repo: repo, inv: inv, reg: reg}
}

func (s *Service) Create(ctx context.Context, p CreateParams) (*Product, error) {
	price := p.Price
	if price.Currency == "" {
		price.Currency = defaultCurrency
	}

	prod := &Product{
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           slug.MakeOrFallback(p.Name, "product-"+uuid.New().String()[:8]),
		Description:    p.Description,
		Price:          price,
		CompareAtPrice: denominateLike(p.CompareAtPrice, price),
		SKU:            p.SKU,
		Status:         StatusDraft,
	}

	if p.Status != "" {
		prod.Status = p.Status
	}

	if err := s.repo.Create(ctx, prod); err != nil {
		return nil, err
	}

	if err := s.reg.EnsureLevel(ctx, prod.ID); err != nil {
		return nil, err
	}

	// (0,0) by construction: EnsureLevel just wrote the row and nothing can hold a
	// reservation yet, so reading inventory back would be a round trip for a
	// known value.
	prod.Availability = Availability{}
	return prod, nil
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

// GetByIDsIncludingDeleted returns products regardless of status or deleted_at,
// so a consumer holding a stale id (a cart line, a wishlist entry) can render
// what it has instead of dropping the row.
func (s *Service) GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]Product, error) {
	products, err := s.repo.GetByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, err
	}
	return products, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, p UpdateParams) (*Product, error) {
	prod, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.CategoryID != nil {
		prod.CategoryID = p.CategoryID
	}
	if p.Name != nil {
		prod.Name = *p.Name
		prod.Slug = slug.MakeOrFallback(prod.Name, "product-"+prod.ID.String()[:8])
	}
	if p.Description != nil {
		prod.Description = p.Description
	}
	if p.Price != nil {
		prod.Price = *p.Price
		// The *stored* compare-at price too, or it keeps the old currency. The branch
		// below overwrites this if the caller supplied one.
		prod.CompareAtPrice = denominateLike(prod.CompareAtPrice, prod.Price)
	}
	if p.CompareAtPrice != nil {
		prod.CompareAtPrice = denominateLike(p.CompareAtPrice, prod.Price)
	}
	if p.SKU != nil {
		prod.SKU = p.SKU
	}
	if p.Status != nil {
		prod.Status = *p.Status
	}

	if err := s.repo.Update(ctx, prod); err != nil {
		return nil, err
	}

	return prod, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) AddImage(ctx context.Context, productID uuid.UUID, p AddImageParams) (*Image, error) {
	if _, err := s.repo.GetByID(ctx, productID); err != nil {
		return nil, err
	}

	img := &Image{
		ProductID: productID,
		URL:       p.URL,
		AltText:   p.AltText,
	}
	if p.SortOrder != nil {
		img.SortOrder = *p.SortOrder
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

// CountPublishedByCategory backs category's ProductCounter port: category has no
// products access of its own.
func (s *Service) CountPublishedByCategory(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return s.repo.CountPublishedByCategory(ctx, categoryID)
}

// One call per page, and a product with no level row reads as zero rather than
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
