package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

type Reader struct {
	repo Repository
	inv  InventoryReader
}

func New(repo Repository, inv InventoryReader) *Reader {
	return &Reader{repo: repo, inv: inv}
}

func (r *Reader) GetBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	p, err := r.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	if p.Status != domain.StatusPublished {
		return nil, apperror.ErrNotFound
	}

	images, err := r.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	one := []domain.Product{*p}
	if err := r.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (r *Reader) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	p, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	images, err := r.repo.GetImagesByProductID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	one := []domain.Product{*p}
	if err := r.enrich(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (r *Reader) ListPublished(
	ctx context.Context,
	params PublishedListParams,
) ([]domain.Product, string, bool, error) {
	products, nextCursor, hasMore, err := r.repo.ListPublished(ctx, params)
	if err != nil {
		return nil, "", false, err
	}
	if err := r.enrich(ctx, products); err != nil {
		return nil, "", false, err
	}
	return products, nextCursor, hasMore, nil
}

func (r *Reader) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Product, int, error) {
	products, total, err := r.repo.ListAdmin(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if err := r.enrich(ctx, products); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// GetByIDsIncludingDeleted returns products regardless of status or deleted_at,
// so a consumer holding a stale id (a cart line, a wishlist entry) can render
// what it has instead of dropping the row.
func (r *Reader) GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error) {
	products, err := r.repo.GetByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, products); err != nil {
		return nil, err
	}
	return products, nil
}

// CountPublished backs category's ProductCounter port: category has no
// products access of its own.
func (r *Reader) CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.repo.CountPublishedByCategory(ctx, categoryID)
}

// GetInfo backs cart's ProductLookup port for a single product.
func (r *Reader) GetInfo(ctx context.Context, id uuid.UUID) (*contract.Product, error) {
	p, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &contract.Product{
		ID:        p.ID,
		Name:      p.Name,
		Price:     p.Price,
		Status:    p.Status,
		Available: p.Availability.Available,
	}, nil
}

// GetInfoByIDs backs cart's ProductLookup port for a whole cart at once.
func (r *Reader) GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]contract.Product, error) {
	products, err := r.GetByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]contract.Product, len(products))
	for _, p := range products {
		status := p.Status
		if p.DeletedAt != nil {
			// remove only sets deleted_at, so a withdrawn product still reads
			// status='published'. Forwarding that would make the line look sellable
			// to cart and to order's availability guard.
			status = "unavailable"
		}
		out[p.ID] = contract.Product{
			ID:        p.ID,
			Name:      p.Name,
			Price:     p.Price,
			Status:    status,
			Available: p.Availability.Available,
		}
	}
	return out, nil
}

// One call per page, and a product with no level row reads as zero rather than
// erroring, so a missing row cannot take down a listing.
func (r *Reader) enrich(ctx context.Context, products []domain.Product) error {
	if len(products) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(products))
	for i := range products {
		ids[i] = products[i].ID
	}
	levels, err := r.inv.GetAvailability(ctx, ids)
	if err != nil {
		return fmt.Errorf("reading availability: %w", err)
	}
	for i := range products {
		products[i].Availability = levels[products[i].ID]
	}
	return nil
}
