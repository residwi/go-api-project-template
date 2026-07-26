package promotion

import (
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypePercentage  Type = "percentage"
	TypeFixedAmount Type = "fixed_amount"
)

type Promotion struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"`
	Type           Type      `json:"type"`
	Value          int64     `json:"value"`
	MinOrderAmount int64     `json:"min_order_amount"`
	MaxDiscount    *int64    `json:"max_discount,omitempty"`
	MaxUses        *int      `json:"max_uses,omitempty"`
	UsedCount      int       `json:"used_count"`
	StartsAt       time.Time `json:"starts_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CouponUsage struct {
	ID        uuid.UUID `json:"id"`
	CouponID  uuid.UUID `json:"coupon_id"`
	UserID    uuid.UUID `json:"user_id"`
	OrderID   uuid.UUID `json:"order_id"`
	Discount  int64     `json:"discount"`
	CreatedAt time.Time `json:"created_at"`
}
