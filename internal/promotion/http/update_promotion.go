package http

import (
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/promotion"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type updatePromotionRequest struct {
	Code           string         `json:"code" validate:"omitempty,min=1,max=50"`
	Type           promotion.Type `json:"type" validate:"omitempty,oneof=percentage fixed_amount"`
	Value          *int64         `json:"value" validate:"omitempty,min=1"`
	MinOrderAmount *int64         `json:"min_order_amount" validate:"omitempty,min=0"`
	MaxDiscount    *int64         `json:"max_discount"`
	MaxUses        *int           `json:"max_uses"`
	StartsAt       *time.Time     `json:"starts_at"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	Active         *bool          `json:"active"`
}

func (r updatePromotionRequest) toUpdateParams() promotion.UpdateParams {
	return promotion.UpdateParams{
		Code:           r.Code,
		Type:           r.Type,
		Value:          r.Value,
		MinOrderAmount: r.MinOrderAmount,
		MaxDiscount:    r.MaxDiscount,
		MaxUses:        r.MaxUses,
		StartsAt:       r.StartsAt,
		ExpiresAt:      r.ExpiresAt,
		Active:         r.Active,
	}
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updatePromotionRequest](w, r, h.validator)
	if !ok {
		return
	}

	promo, err := h.service.Update(r.Context(), id, req.toUpdateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminPromotionResponse(promo))
}
