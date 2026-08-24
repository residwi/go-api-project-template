package http

import (
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type createPromotionRequest struct {
	Code           string      `json:"code"             validate:"required,min=1,max=50"`
	Type           domain.Type `json:"type"             validate:"required,oneof=percentage fixed_amount"`
	Value          int64       `json:"value"            validate:"required,min=1"`
	MinOrderAmount int64       `json:"min_order_amount" validate:"min=0"`
	MaxDiscount    *int64      `json:"max_discount"     validate:"omitempty,min=1"`
	MaxUses        *int        `json:"max_uses"         validate:"omitempty,min=1"`
	StartsAt       time.Time   `json:"starts_at"        validate:"required"`
	ExpiresAt      time.Time   `json:"expires_at"       validate:"required,gtfield=StartsAt"`
	Active         bool        `json:"active"`
}

type updatePromotionRequest struct {
	Code           string      `json:"code"             validate:"omitempty,min=1,max=50"`
	Type           domain.Type `json:"type"             validate:"omitempty,oneof=percentage fixed_amount"`
	Value          *int64      `json:"value"            validate:"omitempty,min=1"`
	MinOrderAmount *int64      `json:"min_order_amount" validate:"omitempty,min=0"`
	MaxDiscount    *int64      `json:"max_discount"`
	MaxUses        *int        `json:"max_uses"`
	StartsAt       *time.Time  `json:"starts_at"`
	ExpiresAt      *time.Time  `json:"expires_at"`
	Active         *bool       `json:"active"`
}
