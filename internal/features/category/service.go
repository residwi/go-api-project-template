package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo     Repository
	products ProductCounter
	tracer   trace.Tracer
}

func New(repo Repository, products ProductCounter) *Service {
	return &Service{
		repo:     repo,
		products: products,
		tracer:   otel.Tracer("github.com/residwi/go-api-project-template/internal/features/category"),
	}
}

func (s *Service) Create(
	ctx context.Context,
	name string,
	description *string,
	parentID *uuid.UUID,
	sortOrder *int,
	active *bool,
) (_ *domain.Category, err error) {
	ctx, span := s.tracer.Start(ctx, "category.Create")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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
) (_ *domain.Category, err error) {
	ctx, span := s.tracer.Start(ctx, "category.Update")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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

func (s *Service) Delete(ctx context.Context, id uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "category.Delete")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	count, err := s.products.CountPublished(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category has %d published products", errs.ErrBadRequest, count)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) (_ []domain.Category, err error) {
	ctx, span := s.tracer.Start(ctx, "category.List")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.List(ctx)
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (_ *domain.Category, err error) {
	ctx, span := s.tracer.Start(ctx, "category.GetBySlug")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.GetBySlug(ctx, slug)
}
