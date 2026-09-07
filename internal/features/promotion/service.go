package promotion

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo   Repository
	tx     database.TxRunner
	tracer trace.Tracer
}

func New(repo Repository, tx database.TxRunner) *Service {
	return &Service{
		repo:   repo,
		tx:     tx,
		tracer: otel.Tracer("github.com/residwi/go-api-project-template/internal/features/promotion"),
	}
}

func (s *Service) Apply(ctx context.Context, code string, orderAmount int64) (_ int64, err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Apply")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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
) (_ *domain.Promotion, err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Create")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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
) (_ *domain.Promotion, err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Update")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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

func (s *Service) Delete(ctx context.Context, id uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Delete")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Delete(ctx, id)
}

func (s *Service) ListAdmin(
	ctx context.Context,
	params AdminListParams,
) (_ []domain.Promotion, _ int, err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.ListAdmin")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) Reserve(
	ctx context.Context,
	code string,
	userID, orderID uuid.UUID,
	orderSubtotal int64,
) (_ int64, err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Reserve")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	var discountAmount int64

	err = s.tx.Run(ctx, func(txCtx context.Context) error {
		promo, txErr := s.repo.GetByCode(txCtx, code)
		if txErr != nil {
			return txErr
		}

		if txErr := domain.ValidatePromotion(promo, orderSubtotal); txErr != nil {
			return txErr
		}

		discountAmount = domain.ComputeDiscount(promo, orderSubtotal)

		if txErr := s.repo.ApplyPromotion(txCtx, promo.ID); txErr != nil {
			return txErr
		}

		usage := &domain.CouponUsage{
			CouponID: promo.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: discountAmount,
		}
		return s.repo.CreateUsage(txCtx, usage)
	})

	return discountAmount, err
}

func (s *Service) Release(ctx context.Context, orderID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "promotion.Release")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.tx.Run(ctx, func(txCtx context.Context) error {
		usage, txErr := s.repo.DeleteUsageByOrderID(txCtx, orderID)
		if txErr != nil {
			if errors.Is(txErr, errs.ErrNotFound) {
				return nil
			}
			return txErr
		}

		return s.repo.ReleasePromotion(txCtx, usage.CouponID)
	})
}
