package update

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Params struct {
	Name        *string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   *int
	Active      *bool
}

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
		if err := domain.ValidateParentSelf(*cat.ParentID, cat.ID); err != nil {
			return nil, err
		}

		depth, formsCycle, err := c.repo.AncestorDepthAndCycle(ctx, *cat.ParentID, cat.ID, domain.MaxDepth)
		if err != nil {
			return nil, fmt.Errorf("validating parent: %w", err)
		}
		if err := domain.ValidateParentDepth(depth, formsCycle); err != nil {
			return nil, err
		}
	}

	if err := c.repo.Update(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}
