package create

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Params struct {
	Name        string
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
		if err := domain.ValidateParentSelf(*cat.ParentID, uuid.Nil); err != nil {
			return nil, err
		}

		depth, formsCycle, err := c.repo.AncestorDepthAndCycle(ctx, *cat.ParentID, uuid.Nil, domain.MaxDepth)
		if err != nil {
			return nil, fmt.Errorf("validating parent: %w", err)
		}
		if err := domain.ValidateParentDepth(depth, formsCycle); err != nil {
			return nil, err
		}
	}

	if err := c.repo.Create(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}
