package promotion

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Service struct {
	repo Repository
	tx   database.TxRunner
}

func New(repo Repository, tx database.TxRunner) *Service {
	return &Service{repo: repo, tx: tx}
}

// Apply previews a coupon's discount without reserving it. Reserve does
// both, for the order that actually spends the coupon.
func (s *Service) Apply(ctx context.Context, code string, orderAmount int64) (int64, error) {
	promo, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, err
	}

	if err := domain.ValidatePromotion(promo, orderAmount); err != nil {
		return 0, err
	}

	return domain.ComputeDiscount(promo, orderAmount), nil
}

func (s *Service) Create(
	ctx context.Context,
	code string,
	promoType domain.Type,
	value int64,
	minOrderAmount int64,
	maxDiscount *int64,
	maxUses *int,
	startsAt time.Time,
	expiresAt time.Time,
	active bool,
) (*domain.Promotion, error) {
	promo := &domain.Promotion{
		Code:           code,
		Type:           promoType,
		Value:          value,
		MinOrderAmount: minOrderAmount,
		MaxDiscount:    maxDiscount,
		MaxUses:        maxUses,
		StartsAt:       startsAt,
		ExpiresAt:      expiresAt,
		Active:         active,
	}

	if err := domain.ValidatePercentageValue(promo.Type, promo.Value); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	code string,
	promoType domain.Type,
	value *int64,
	minOrderAmount *int64,
	maxDiscount *int64,
	maxUses *int,
	startsAt *time.Time,
	expiresAt *time.Time,
	active *bool,
) (*domain.Promotion, error) {
	promo, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if code != "" {
		promo.Code = code
	}
	if promoType != "" {
		promo.Type = promoType
	}
	if value != nil {
		promo.Value = *value
	}
	if minOrderAmount != nil {
		promo.MinOrderAmount = *minOrderAmount
	}
	if maxDiscount != nil {
		promo.MaxDiscount = maxDiscount
	}
	if maxUses != nil {
		promo.MaxUses = maxUses
	}
	if startsAt != nil {
		promo.StartsAt = *startsAt
	}
	if expiresAt != nil {
		promo.ExpiresAt = *expiresAt
	}
	if active != nil {
		promo.Active = *active
	}

	if err := domain.ValidatePercentageValue(promo.Type, promo.Value); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Promotion, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

// Reserve and Release are bound by name-match into order.CouponReserver
// (both methods) and payment.CouponReleaser (Release alone); their signatures
// must stay byte-identical to what those two ports declare.
func (s *Service) Reserve(
	ctx context.Context,
	code string,
	userID, orderID uuid.UUID,
	orderSubtotal int64,
) (int64, error) {
	var discountAmount int64

	err := s.tx.Run(ctx, func(ctx context.Context) error {
		promo, err := s.repo.GetByCode(ctx, code)
		if err != nil {
			return err
		}

		if err := domain.ValidatePromotion(promo, orderSubtotal); err != nil {
			return err
		}

		discountAmount = domain.ComputeDiscount(promo, orderSubtotal)

		if err := s.repo.ApplyPromotion(ctx, promo.ID); err != nil {
			return err
		}

		usage := &domain.CouponUsage{
			CouponID: promo.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: discountAmount,
		}
		return s.repo.CreateUsage(ctx, usage)
	})

	return discountAmount, err
}

func (s *Service) Release(ctx context.Context, orderID uuid.UUID) error {
	return s.tx.Run(ctx, func(ctx context.Context) error {
		usage, err := s.repo.DeleteUsageByOrderID(ctx, orderID)
		if err != nil {
			if errors.Is(err, apperror.ErrNotFound) {
				return nil
			}
			return err
		}

		return s.repo.ReleasePromotion(ctx, usage.CouponID)
	})
}
