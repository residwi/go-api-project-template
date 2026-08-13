package remove

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

type Command struct {
	repo     Repository
	products ProductCounter
}

func New(repo Repository, products ProductCounter) *Command {
	return &Command{repo: repo, products: products}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID) error {
	count, err := c.products.CountPublished(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category has %d published products", apperror.ErrBadRequest, count)
	}

	return c.repo.Delete(ctx, id)
}
