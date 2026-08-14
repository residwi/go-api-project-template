package reserve

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Repository interface {
	GetByCode(ctx context.Context, code string) (*domain.Promotion, error)
	ApplyPromotion(ctx context.Context, id uuid.UUID) error
	ReleasePromotion(ctx context.Context, id uuid.UUID) error
	CreateUsage(ctx context.Context, usage *domain.CouponUsage) error
	DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.CouponUsage, error)
}
