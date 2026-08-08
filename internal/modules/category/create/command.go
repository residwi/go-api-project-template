package create

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Params struct {
	Name        string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   *int
	Active      *bool
}

// Command takes no TxRunner: it writes one row through its own repository and
// asks nothing else.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, p Params) (*domain.Category, error) {
	cat := &domain.Category{
		Name:        p.Name,
		Slug:        domain.Slugify(p.Name, uuid.New().String()[:8]),
		Description: p.Description,
		ParentID:    p.ParentID,
		Active:      true,
	}

	if p.SortOrder != nil {
		cat.SortOrder = *p.SortOrder
	}
	if p.Active != nil {
		cat.Active = *p.Active
	}

	if cat.ParentID != nil {
		if err := c.validateParent(ctx, *cat.ParentID, uuid.Nil); err != nil {
			return nil, err
		}
	}

	if err := c.repo.Create(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}

func (c *Command) validateParent(ctx context.Context, parentID, selfID uuid.UUID) error {
	if parentID == selfID && selfID != uuid.Nil {
		return fmt.Errorf("%w: category cannot be its own parent", apperror.ErrBadRequest)
	}

	depth, formsCycle, err := c.repo.AncestorDepthAndCycle(ctx, parentID, selfID, domain.MaxDepth)
	if err != nil {
		return fmt.Errorf("validating parent: %w", err)
	}

	if depth == 0 {
		return fmt.Errorf("%w: parent category not found", apperror.ErrBadRequest)
	}
	if formsCycle {
		return fmt.Errorf("%w: circular parent reference", apperror.ErrBadRequest)
	}
	// depth is the distance from parent to root. Adding this child makes it depth+1.
	if depth+1 > domain.MaxDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", apperror.ErrBadRequest, domain.MaxDepth)
	}

	return nil
}
