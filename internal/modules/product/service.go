package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

const defaultCurrency = "USD"

type Deps struct {
	Repo Repository

	InventoryReader    InventoryReader
	InventoryRegistrar InventoryRegistrar
}

type Service struct {
	repo Repository

	inv InventoryReader
	reg InventoryRegistrar
}

func New(d Deps) *Service {
	return &Service{repo: d.Repo, inv: d.InventoryReader, reg: d.InventoryRegistrar}
}

func denominateLike(amount *money.Money, price money.Money) *money.Money {
	if amount == nil {
		return nil
	}
	restated := money.New(amount.Amount, price.Currency)
	return &restated
}

func (s *Service) Create(
	ctx context.Context,
	categoryID *uuid.UUID,
	name string,
	description *string,
	price money.Money,
	compareAtPrice *money.Money,
	sku *string,
	status string,
) (*domain.Product, error) {
	if price.Currency == "" {
		price.Currency = defaultCurrency
	}

	prod := &domain.Product{
		CategoryID:     categoryID,
		Name:           name,
		Slug:           slug.MakeOrFallback(name, "product-"+uuid.New().String()[:8]),
		Description:    description,
		Price:          price,
		CompareAtPrice: denominateLike(compareAtPrice, price),
		SKU:            sku,
		Status:         domain.StatusDraft,
	}

	if status != "" {
		prod.Status = status
	}

	if err := s.repo.Create(ctx, prod); err != nil {
		return nil, err
	}

	if err := s.reg.EnsureLevel(ctx, prod.ID); err != nil {
		return nil, err
	}

	prod.Availability = inventorycontract.Availability{}
	return prod, nil
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	categoryID *uuid.UUID,
	name *string,
	description *string,
	price *money.Money,
	compareAtPrice *money.Money,
	sku *string,
	status *string,
) (*domain.Product, error) {
	prod, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if categoryID != nil {
		prod.CategoryID = categoryID
	}
	if name != nil {
		prod.Name = *name
		prod.Slug = slug.MakeOrFallback(prod.Name, "product-"+prod.ID.String()[:8])
	}
	if description != nil {
		prod.Description = description
	}
	if price != nil {
		prod.Price = *price
		prod.CompareAtPrice = denominateLike(prod.CompareAtPrice, prod.Price)
	}
	if compareAtPrice != nil {
		prod.CompareAtPrice = denominateLike(compareAtPrice, prod.Price)
	}
	if sku != nil {
		prod.SKU = sku
	}
	if status != nil {
		prod.Status = *status
	}

	if err := s.repo.Update(ctx, prod); err != nil {
		return nil, err
	}

	return prod, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) AddImage(
	ctx context.Context,
	productID uuid.UUID,
	url string,
	altText *string,
	sortOrder *int,
) (*domain.Image, error) {
	if _, err := s.repo.GetByID(ctx, productID); err != nil {
		return nil, err
	}

	img := &domain.Image{
		ProductID: productID,
		URL:       url,
		AltText:   altText,
	}
	if sortOrder != nil {
		img.SortOrder = *sortOrder
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

func (s *Service) GetBySlug(ctx context.Context, productSlug string) (*domain.Product, error) {
	p, err := s.repo.GetBySlug(ctx, productSlug)
	if err != nil {
		return nil, err
	}

	if p.Status != domain.StatusPublished {
		return nil, apperror.ErrNotFound
	}

	images, err := s.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	one := []domain.Product{*p}
	if err := s.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	images, err := s.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	one := []domain.Product{*p}
	if err := s.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (s *Service) ListPublished(
	ctx context.Context,
	params PublishedListParams,
) ([]domain.Product, string, bool, error) {
	products, nextCursor, hasMore, err := s.repo.ListPublished(ctx, params)
	if err != nil {
		return nil, "", false, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, "", false, err
	}
	return products, nextCursor, hasMore, nil
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Product, int, error) {
	products, total, err := s.repo.ListAdmin(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (s *Service) CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return s.repo.CountPublishedByCategory(ctx, categoryID)
}

func (s *Service) GetInfo(ctx context.Context, id uuid.UUID) (*ProductInfo, error) {
	p, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &ProductInfo{
		ID:        p.ID,
		Name:      p.Name,
		Price:     p.Price,
		Status:    p.Status,
		Available: p.Availability.Available,
	}, nil
}

func (s *Service) GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductInfo, error) {
	products, err := s.getByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]ProductInfo, len(products))
	for _, p := range products {
		status := p.Status
		if p.DeletedAt != nil {
			status = "unavailable"
		}
		out[p.ID] = ProductInfo{
			ID:        p.ID,
			Name:      p.Name,
			Price:     p.Price,
			Status:    status,
			Available: p.Availability.Available,
		}
	}
	return out, nil
}

// getByIDsIncludingDeleted is unexported: GetInfoByIDs is its only caller,
// inside this package, and nothing outside it has ever named the exported
// form.
func (s *Service) getByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error) {
	products, err := s.repo.GetByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, products); err != nil {
		return nil, err
	}
	return products, nil
}

func (s *Service) enrich(ctx context.Context, products []domain.Product) error {
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
