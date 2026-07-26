package promotion

import (
	"time"

	"github.com/google/uuid"
)

type ApplyRequest struct {
	Code     string `json:"code" validate:"required"`
	Subtotal int64  `json:"subtotal" validate:"required,min=1"`
}

type ApplyResponse struct {
	Code     string `json:"code"`
	Discount int64  `json:"discount"`
}

type CreateRequest struct {
	Code           string    `json:"code" validate:"required,min=1,max=50"`
	Type           Type      `json:"type" validate:"required,oneof=percentage fixed_amount"`
	Value          int64     `json:"value" validate:"required,min=1"`
	MinOrderAmount int64     `json:"min_order_amount" validate:"min=0"`
	MaxDiscount    *int64    `json:"max_discount" validate:"omitempty,min=1"`
	MaxUses        *int      `json:"max_uses" validate:"omitempty,min=1"`
	StartsAt       time.Time `json:"starts_at" validate:"required"`
	ExpiresAt      time.Time `json:"expires_at" validate:"required,gtfield=StartsAt"`
	Active         bool      `json:"active"`
}

type UpdateRequest struct {
	Code           string     `json:"code" validate:"omitempty,min=1,max=50"`
	Type           Type       `json:"type" validate:"omitempty,oneof=percentage fixed_amount"`
	Value          *int64     `json:"value" validate:"omitempty,min=1"`
	MinOrderAmount *int64     `json:"min_order_amount" validate:"omitempty,min=0"`
	MaxDiscount    *int64     `json:"max_discount"`
	MaxUses        *int       `json:"max_uses"`
	StartsAt       *time.Time `json:"starts_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Active         *bool      `json:"active"`
}

type ListParams struct {
	Page     int
	PageSize int
}

type AdminCreateResponse struct {
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
