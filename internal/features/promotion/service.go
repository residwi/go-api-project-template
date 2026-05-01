package promotion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Service struct {
	repo Repository
	pool *pgxpool.Pool
}

func NewService(repo Repository, pool *pgxpool.Pool) *Service {
	return &Service{repo: repo, pool: pool}
}

func (s *Service) Validate(ctx context.Context, code string, orderAmount int64) (int64, error) {
	promo, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, err
	}

	if err := validatePromotion(promo, orderAmount); err != nil {
		return 0, err
	}

	return computeDiscount(promo, orderAmount), nil
}

func (s *Service) Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error) {
	var discountAmount int64

	err := database.WithTx(ctx, s.pool, func(ctx context.Context) error {
		promo, err := s.repo.GetByCode(ctx, code)
		if err != nil {
			return err
		}

		if err := validatePromotion(promo, orderSubtotal); err != nil {
			return err
		}

		discountAmount = computeDiscount(promo, orderSubtotal)

		if err := s.repo.ApplyPromotion(ctx, promo.ID); err != nil {
			return err
		}

		usage := &CouponUsage{
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
	return database.WithTx(ctx, s.pool, func(ctx context.Context) error {
		usage, err := s.repo.DeleteUsageByOrderID(ctx, orderID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil
			}
			return err
		}

		return s.repo.ReleasePromotion(ctx, usage.CouponID)
	})
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Promotion, error) {
	promo := &Promotion{
		Code:           req.Code,
		Type:           req.Type,
		Value:          req.Value,
		MinOrderAmount: req.MinOrderAmount,
		MaxDiscount:    req.MaxDiscount,
		MaxUses:        req.MaxUses,
		StartsAt:       req.StartsAt,
		ExpiresAt:      req.ExpiresAt,
		Active:         req.Active,
	}

	if err := s.repo.Create(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Promotion, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Promotion, error) {
	promo, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Code != "" {
		promo.Code = req.Code
	}
	if req.Type != "" {
		promo.Type = req.Type
	}
	if req.Value != nil {
		promo.Value = *req.Value
	}
	if req.MinOrderAmount != nil {
		promo.MinOrderAmount = *req.MinOrderAmount
	}
	if req.MaxDiscount != nil {
		promo.MaxDiscount = req.MaxDiscount
	}
	if req.MaxUses != nil {
		promo.MaxUses = req.MaxUses
	}
	if req.StartsAt != nil {
		promo.StartsAt = *req.StartsAt
	}
	if req.ExpiresAt != nil {
		promo.ExpiresAt = *req.ExpiresAt
	}
	if req.Active != nil {
		promo.Active = *req.Active
	}

	if err := s.repo.Update(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func validatePromotion(promo *Promotion, orderAmount int64) error {
	if !promo.Active {
		return fmt.Errorf("%w: promotion is not active", core.ErrBadRequest)
	}

	now := time.Now()
	if now.Before(promo.StartsAt) {
		return fmt.Errorf("%w: promotion has not started yet", core.ErrBadRequest)
	}
	if now.After(promo.ExpiresAt) {
		return fmt.Errorf("%w: promotion has expired", core.ErrBadRequest)
	}

	if promo.MaxUses != nil && promo.UsedCount >= *promo.MaxUses {
		return core.ErrCouponExhausted
	}

	if orderAmount < promo.MinOrderAmount {
		return fmt.Errorf("%w: order amount below minimum", core.ErrBadRequest)
	}

	return nil
}

const percentDivisor = 100.0

func computeDiscount(promo *Promotion, orderSubtotal int64) int64 {
	var discount int64

	switch promo.Type {
	case TypePercentage:
		discount = int64(math.Floor(float64(orderSubtotal) * float64(promo.Value) / percentDivisor))
		if promo.MaxDiscount != nil && discount > *promo.MaxDiscount {
			discount = *promo.MaxDiscount
		}
	case TypeFixedAmount:
		discount = promo.Value
	}

	if discount > orderSubtotal {
		discount = orderSubtotal
	}

	return discount
}
