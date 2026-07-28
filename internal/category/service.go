package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

const maxCategoryDepth = 5

type Service struct {
	repo     Repository
	products ProductCounter
}

func NewService(repo Repository, products ProductCounter) *Service {
	return &Service{repo: repo, products: products}
}

func (s *Service) Create(ctx context.Context, p CreateParams) (*Category, error) {
	cat := &Category{
		Name:        p.Name,
		Slug:        slug.MakeOrFallback(p.Name, "category-"+uuid.New().String()[:8]),
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
		if err := s.validateParent(ctx, *cat.ParentID, uuid.Nil); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, cat); err != nil {
		return nil, err
	}

	return cat, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*Category, error) {
	return s.repo.GetBySlug(ctx, slug)
}

func (s *Service) List(ctx context.Context) ([]Category, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Category, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, p UpdateParams) (*Category, error) {
	cat, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.Name != nil {
		cat.Name = *p.Name
		cat.Slug = slug.MakeOrFallback(cat.Name, "category-"+cat.ID.String()[:8])
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
		if err := s.validateParent(ctx, *cat.ParentID, cat.ID); err != nil {
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

// validateParent checks that parent_id exists, does not create a cycle, and max depth is 5.
func (s *Service) validateParent(ctx context.Context, parentID, selfID uuid.UUID) error {
	if parentID == selfID && selfID != uuid.Nil {
		return fmt.Errorf("%w: category cannot be its own parent", apperror.ErrBadRequest)
	}

	depth, formsCycle, err := s.repo.AncestorDepthAndCycle(ctx, parentID, selfID, maxCategoryDepth)
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
	if depth+1 > maxCategoryDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", apperror.ErrBadRequest, maxCategoryDepth)
	}

	return nil
}
