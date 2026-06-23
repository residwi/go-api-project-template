package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

const maxCategoryDepth = 5

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req CreateCategoryRequest) (*Category, error) {
	cat := &Category{
		Name:        req.Name,
		Slug:        core.SlugifyOrFallback(req.Name, "category-"+uuid.New().String()[:8]),
		Description: req.Description,
		ParentID:    req.ParentID,
		Active:      true,
	}

	if req.SortOrder != nil {
		cat.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		cat.Active = *req.Active
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

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateCategoryRequest) (*Category, error) {
	cat, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		cat.Name = *req.Name
		cat.Slug = core.SlugifyOrFallback(cat.Name, "category-"+cat.ID.String()[:8])
	}
	if req.Description != nil {
		cat.Description = req.Description
	}
	if req.ParentID != nil {
		cat.ParentID = req.ParentID
	}
	if req.SortOrder != nil {
		cat.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		cat.Active = *req.Active
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
	count, err := s.repo.CountPublishedProducts(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category has %d published products", core.ErrBadRequest, count)
	}

	return s.repo.Delete(ctx, id)
}

// validateParent checks that parent_id exists, does not create a cycle, and max depth is 5.
func (s *Service) validateParent(ctx context.Context, parentID, selfID uuid.UUID) error {
	if parentID == selfID && selfID != uuid.Nil {
		return fmt.Errorf("%w: category cannot be its own parent", core.ErrBadRequest)
	}

	depth, formsCycle, err := s.repo.AncestorDepthAndCycle(ctx, parentID, selfID, maxCategoryDepth)
	if err != nil {
		return fmt.Errorf("validating parent: %w", err)
	}

	if depth == 0 {
		return fmt.Errorf("%w: parent category not found", core.ErrBadRequest)
	}
	if formsCycle {
		return fmt.Errorf("%w: circular parent reference", core.ErrBadRequest)
	}
	// depth is the distance from parent to root. Adding this child makes it depth+1.
	if depth+1 > maxCategoryDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", core.ErrBadRequest, maxCategoryDepth)
	}

	return nil
}
