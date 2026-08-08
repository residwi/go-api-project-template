// Package domain holds promotion's aggregate and its rules. It is
// module-private: what leaves promotion leaves through a slice's return type.
package domain

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

type Type string

const (
	TypePercentage  Type = "percentage"
	TypeFixedAmount Type = "fixed_amount"
)

type Promotion struct {
	ID             uuid.UUID
	Code           string
	Type           Type
	Value          int64
	MinOrderAmount int64
	MaxDiscount    *int64
	MaxUses        *int
	UsedCount      int
	StartsAt       time.Time
	ExpiresAt      time.Time
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CouponUsage struct {
	ID        uuid.UUID
	CouponID  uuid.UUID
	UserID    uuid.UUID
	OrderID   uuid.UUID
	Discount  int64
	CreatedAt time.Time
}

// ValidatePercentageValue rejects a percentage promotion above 100: above
// that, ComputeDiscount's clamp would silently make the order free. The cap
// is type-conditional -- fixed_amount allows any positive value -- so no
// validate tag can express it. create and update both call this once they
// hold the final, persisted combination of Type and Value.
func ValidatePercentageValue(promoType Type, value int64) error {
	if promoType == TypePercentage && value > 100 {
		return fmt.Errorf("%w: percentage discount value cannot exceed 100", apperror.ErrBadRequest)
	}
	return nil
}

// ValidatePromotion checks a promotion against the order amount it is being
// applied to. apply and reserve both call this immediately after loading the
// promotion by code, before computing a discount.
func ValidatePromotion(promo *Promotion, orderAmount int64) error {
	if !promo.Active {
		return fmt.Errorf("%w: promotion is not active", apperror.ErrBadRequest)
	}

	now := time.Now()
	if now.Before(promo.StartsAt) {
		return fmt.Errorf("%w: promotion has not started yet", apperror.ErrBadRequest)
	}
	if now.After(promo.ExpiresAt) {
		return fmt.Errorf("%w: promotion has expired", apperror.ErrBadRequest)
	}

	if promo.MaxUses != nil && promo.UsedCount >= *promo.MaxUses {
		return apperror.ErrCouponExhausted
	}

	if orderAmount < promo.MinOrderAmount {
		return fmt.Errorf("%w: order amount below minimum", apperror.ErrBadRequest)
	}

	return nil
}

const percentDivisor = 100.0

// ComputeDiscount turns a validated promotion into the amount to take off
// orderSubtotal. apply and reserve both call this once ValidatePromotion
// has passed.
func ComputeDiscount(promo *Promotion, orderSubtotal int64) int64 {
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

	// Safety net for a fixed_amount promo that legitimately exceeds the subtotal;
	// percentage overflows are rejected at write time (ValidatePercentageValue).
	if discount > orderSubtotal {
		discount = orderSubtotal
	}

	return discount
}
