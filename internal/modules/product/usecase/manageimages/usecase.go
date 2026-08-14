package manageimages

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

type Params struct {
	URL       string
	AltText   *string
	SortOrder *int
}

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
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
