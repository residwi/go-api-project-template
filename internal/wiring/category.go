package wiring

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/product"
)

type productCounterAdapter struct{ svc *product.Service }

func (a *productCounterAdapter) CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return a.svc.CountPublishedByCategory(ctx, categoryID)
}

func NewCategoryService(repo category.Repository, productSvc *product.Service) *category.Service {
	return category.NewService(repo, &productCounterAdapter{svc: productSvc})
}
