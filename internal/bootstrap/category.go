package bootstrap

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/modules/product"
)

type productCounterAdapter struct{ svc *product.Service }

func (a *productCounterAdapter) CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return a.svc.CountPublishedByCategory(ctx, categoryID)
}

func NewCategoryService(repo category.Repository, productSvc *product.Service) *category.Service {
	return category.NewService(repo, &productCounterAdapter{svc: productSvc})
}
