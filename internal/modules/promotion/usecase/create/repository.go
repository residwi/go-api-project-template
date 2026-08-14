package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Repository interface {
	Create(ctx context.Context, promo *domain.Promotion) error
}
