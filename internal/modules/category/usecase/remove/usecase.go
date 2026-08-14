package remove

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

type UseCase struct {
	repo     Repository
	products ProductCounter
}

func New(repo Repository, products ProductCounter) *UseCase {
	return &UseCase{repo: repo, products: products}
}

func (c *UseCase) Execute(ctx context.Context, id uuid.UUID) error {
	count, err := c.products.CountPublished(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category has %d published products", apperror.ErrBadRequest, count)
	}

	return c.repo.Delete(ctx, id)
}
