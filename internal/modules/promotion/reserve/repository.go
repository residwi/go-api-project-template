package reserve

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

// Repository is reserve's own storage: promotions plus the coupon_usages row
// a claim writes and a release deletes. Its only implementation is
// reserve/postgres, constructed in promotion/module.go.
type Repository interface {
	GetByCode(ctx context.Context, code string) (*domain.Promotion, error)
	ApplyPromotion(ctx context.Context, id uuid.UUID) error
	ReleasePromotion(ctx context.Context, id uuid.UUID) error
	CreateUsage(ctx context.Context, usage *domain.CouponUsage) error
	DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.CouponUsage, error)
}
