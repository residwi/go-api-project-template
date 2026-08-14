package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

type Repository interface {
	Create(ctx context.Context, p *domain.Product) error
}
