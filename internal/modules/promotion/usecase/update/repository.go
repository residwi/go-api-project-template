package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Promotion, error)
	Update(ctx context.Context, promo *domain.Promotion) error
}
