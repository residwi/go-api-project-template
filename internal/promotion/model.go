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
