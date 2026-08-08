package update

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Params struct {
	Name        *string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   *int
	Active      *bool
}

// Command takes no TxRunner: it loads one row through its own repository,
// patches it and writes it back, with nothing else to ask.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID, p Params) (*domain.Category, error) {
	cat, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.Name != nil {
		cat.Name = *p.Name
		cat.Slug = domain.Slugify(cat.Name, cat.ID.String()[:8])
	}
	if p.Description != nil {
		cat.Description = p.Description
	}
	if p.ParentID != nil {
		cat.ParentID = p.ParentID
	}
	if p.SortOrder != nil {
		cat.SortOrder = *p.SortOrder
	}
	if p.Active != nil {
		cat.Active = *p.Active
	}

	if cat.ParentID != nil {
		if err := c.validateParent(ctx, *cat.ParentID, cat.ID); err != nil {
			return nil, err
		}
	}

	if err := c.repo.Update(ctx, cat); err != nil {
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
