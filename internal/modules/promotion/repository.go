package promotion

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, promo *Promotion) error
	GetByID(ctx context.Context, id uuid.UUID) (*Promotion, error)
	GetByCode(ctx context.Context, code string) (*Promotion, error)
	Update(ctx context.Context, promo *Promotion) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListAdmin(ctx context.Context, params ListParams) ([]Promotion, int, error)
	ApplyPromotion(ctx context.Context, id uuid.UUID) error
	ReleasePromotion(ctx context.Context, id uuid.UUID) error
	CreateUsage(ctx context.Context, usage *CouponUsage) error
	DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*CouponUsage, error)
}
