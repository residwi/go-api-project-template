package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Deps struct {
	Repo     Repository
	Products ProductCounter
}

type Service struct {
	repo     Repository
	products ProductCounter
}

func New(d Deps) *Service {
	return &Service{repo: d.Repo, products: d.Products}
}

func (s *Service) Create(
	ctx context.Context,
	name string,
	description *string,
	parentID *uuid.UUID,
	sortOrder *int,
	active *bool,
) (*domain.Category, error) {
	cat := &domain.Category{
		Name:        name,
		Slug:        domain.Slugify(name, uuid.New().String()[:8]),
		Description: description,
		ParentID:    parentID,
		Active:      true,
	}

	if sortOrder != nil {
		cat.SortOrder = *sortOrder
	}
	if active != nil {
		cat.Active = *active
	}

	if cat.ParentID != nil {
		if err := domain.ValidateParentSelf(*cat.ParentID, uuid.Nil); err != nil {
			return nil, err
		}

		depth, formsCycle, err := s.repo.AncestorDepthAndCycle(ctx, *cat.ParentID, uuid.Nil, domain.MaxDepth)
		if err != nil {
			return nil, fmt.Errorf("validating parent: %w", err)
		}
		if err := domain.ValidateParentDepth(depth, formsCycle); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	name *string,
	description *string,
	parentID *uuid.UUID,
	sortOrder *int,
	active *bool,
) (*domain.Category, error) {
	cat, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != nil {
		cat.Name = *name
		cat.Slug = domain.Slugify(cat.Name, cat.ID.String()[:8])
	}
	if description != nil {
		cat.Description = description
	}
	if parentID != nil {
		cat.ParentID = parentID
	}
	if sortOrder != nil {
		cat.SortOrder = *sortOrder
	}
	if active != nil {
		cat.Active = *active
	}

	if cat.ParentID != nil {
		if err := domain.ValidateParentSelf(*cat.ParentID, cat.ID); err != nil {
			return nil, err
		}

		depth, formsCycle, err := s.repo.AncestorDepthAndCycle(ctx, *cat.ParentID, cat.ID, domain.MaxDepth)
		if err != nil {
			return nil, fmt.Errorf("validating parent: %w", err)
		}
		if err := domain.ValidateParentDepth(depth, formsCycle); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Update(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	count, err := s.products.CountPublished(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category has %d published products", apperror.ErrBadRequest, count)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]domain.Category, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	return s.repo.GetBySlug(ctx, slug)
}
