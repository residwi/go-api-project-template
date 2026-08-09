package images

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

// Params may leave SortOrder nil: Add defaults it to zero.
type Params struct {
	URL       string
	AltText   *string
	SortOrder *int
}

// Command has no route: grep across internal, test and cmd found no caller of
// Add, Delete or AvailableQuantity outside product itself. Kept rather than
// dropped -- deleting a method inside a refactor would hide it, and a slice
// holding unused methods is visible on `ls`.
type Command struct {
	repo Repository
	inv  InventoryReader
}

func New(repo Repository, inv InventoryReader) *Command {
	return &Command{repo: repo, inv: inv}
}

func (c *Command) Add(ctx context.Context, productID uuid.UUID, p Params) (*domain.Image, error) {
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

func (c *Command) Delete(ctx context.Context, productID, imageID uuid.UUID) error {
	if _, err := c.repo.GetByID(ctx, productID); err != nil {
		return err
	}

	return c.repo.DeleteImage(ctx, imageID)
}

func (c *Command) AvailableQuantity(ctx context.Context, id uuid.UUID) (int, error) {
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
