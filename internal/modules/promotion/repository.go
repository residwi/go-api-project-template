package promotion

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	GetByCode(ctx context.Context, code string) (*domain.Promotion, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Promotion, error)
	Create(ctx context.Context, promo *domain.Promotion) error
	Update(ctx context.Context, promo *domain.Promotion) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Promotion, int, error)
	ApplyPromotion(ctx context.Context, id uuid.UUID) error
	ReleasePromotion(ctx context.Context, id uuid.UUID) error
	CreateUsage(ctx context.Context, usage *domain.CouponUsage) error
	DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.CouponUsage, error)
}

type AdminListParams struct {
	paging.OffsetPage
}
