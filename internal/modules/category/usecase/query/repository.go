package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Repository interface {
	List(ctx context.Context) ([]domain.Category, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Category, error)
}
