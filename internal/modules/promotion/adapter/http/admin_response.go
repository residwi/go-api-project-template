package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type adminPromotionResponse struct {
	ID             uuid.UUID   `json:"id"`
	Code           string      `json:"code"`
	Type           domain.Type `json:"type"`
	Value          int64       `json:"value"`
	MinOrderAmount int64       `json:"min_order_amount"`
	MaxDiscount    *int64      `json:"max_discount,omitempty"`
	MaxUses        *int        `json:"max_uses,omitempty"`
	UsedCount      int         `json:"used_count"`
	StartsAt       time.Time   `json:"starts_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
	Active         bool        `json:"active"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func toAdminPromotionResponse(p *domain.Promotion) adminPromotionResponse {
	return adminPromotionResponse{
		ID:             p.ID,
		Code:           p.Code,
		Type:           p.Type,
		Value:          p.Value,
		MinOrderAmount: p.MinOrderAmount,
		MaxDiscount:    p.MaxDiscount,
		MaxUses:        p.MaxUses,
		UsedCount:      p.UsedCount,
		StartsAt:       p.StartsAt,
		ExpiresAt:      p.ExpiresAt,
		Active:         p.Active,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}
