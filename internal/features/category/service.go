package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const maxCategoryDepth = 5

type Service struct {
	repo Repository
	pool *pgxpool.Pool
}

func NewService(repo Repository, pool *pgxpool.Pool) *Service {
	return &Service{repo: repo, pool: pool}
}

func (s *Service) Create(ctx context.Context, req CreateCategoryRequest) (*Category, error) {
	cat := &Category{
		Name:        req.Name,
		Slug:        core.Slugify(req.Name),
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
		cat.Slug = core.Slugify(cat.Name)
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
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}

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

	db := database.DB(ctx, s.pool)

	// Recursive CTE: walk ancestors from parentID up to root, detect cycle and depth.
	query := `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, 1 AS depth
			FROM categories WHERE id = $1
			UNION ALL
			SELECT c.id, c.parent_id, a.depth + 1
			FROM categories c
			JOIN ancestors a ON a.parent_id = c.id
			WHERE a.depth < 6
		)
		SELECT COALESCE(MAX(depth), 0), COUNT(*) FILTER (WHERE id = $2) FROM ancestors`

	var maxDepth int
	var cycleCount int
	err := db.QueryRow(ctx, query, parentID, selfID).Scan(&maxDepth, &cycleCount)
	if err != nil {
		return fmt.Errorf("validating parent: %w", err)
	}

	if maxDepth == 0 {
		return fmt.Errorf("%w: parent category not found", core.ErrBadRequest)
	}
	if cycleCount > 0 {
		return fmt.Errorf("%w: circular parent reference", core.ErrBadRequest)
	}
	// maxDepth is the depth from parent to root. Adding this child makes it maxDepth+1.
	if maxDepth+1 > maxCategoryDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", core.ErrBadRequest, maxCategoryDepth)
	}

	return nil
}
