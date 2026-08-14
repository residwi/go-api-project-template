package images

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

type Params struct {
	URL       string
	AltText   *string
	SortOrder *int
}

type UseCase struct {
	repo Repository
	inv  InventoryReader
}

func New(repo Repository, inv InventoryReader) *UseCase {
	return &UseCase{repo: repo, inv: inv}
}

func (c *UseCase) Add(ctx context.Context, productID uuid.UUID, p Params) (*domain.Image, error) {
	if _, err := c.repo.GetByID(ctx, productID); err != nil {
		return nil, err
	}

	img := &domain.Image{
		ProductID: productID,
		URL:       p.URL,
		AltText:   p.AltText,
	}
	if p.SortOrder != nil {
		img.SortOrder = *p.SortOrder
	}

	if err := c.repo.AddImage(ctx, img); err != nil {
		return nil, err
	}

	return img, nil
}

func (c *UseCase) Delete(ctx context.Context, productID, imageID uuid.UUID) error {
	if _, err := c.repo.GetByID(ctx, productID); err != nil {
		return err
	}

	return c.repo.DeleteImage(ctx, imageID)
}

func (c *UseCase) AvailableQuantity(ctx context.Context, id uuid.UUID) (int, error) {
	if _, err := c.repo.GetByID(ctx, id); err != nil {
		return 0, err
	}
	levels, err := c.inv.GetAvailability(ctx, []uuid.UUID{id})
	if err != nil {
		return 0, err
	}
	avail := levels[id].Available
	if avail < 0 {
		return 0, fmt.Errorf("%w: negative available quantity", apperror.ErrInsufficientStock)
	}
	return avail, nil
}
