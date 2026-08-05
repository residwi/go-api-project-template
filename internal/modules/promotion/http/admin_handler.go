package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *promotion.Service
	validator *validator.Validator
}

type createPromotionRequest struct {
	Code           string         `json:"code"             validate:"required,min=1,max=50"`
	Type           promotion.Type `json:"type"             validate:"required,oneof=percentage fixed_amount"`
	Value          int64          `json:"value"            validate:"required,min=1"`
	MinOrderAmount int64          `json:"min_order_amount" validate:"min=0"`
	MaxDiscount    *int64         `json:"max_discount"     validate:"omitempty,min=1"`
	MaxUses        *int           `json:"max_uses"         validate:"omitempty,min=1"`
	StartsAt       time.Time      `json:"starts_at"        validate:"required"`
	ExpiresAt      time.Time      `json:"expires_at"       validate:"required,gtfield=StartsAt"`
	Active         bool           `json:"active"`
}

func (r createPromotionRequest) toCreateParams() promotion.CreateParams {
	return promotion.CreateParams{
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

// adminPromotionResponse is the admin-only shape of a promotion -- there is
// no public equivalent, since a shopper only ever sees applyResponse's
// computed discount. It carries the internal usage counters and per-user
// limits that applyResponse deliberately withholds.
type adminPromotionResponse struct {
	ID             uuid.UUID      `json:"id"`
	Code           string         `json:"code"`
	Type           promotion.Type `json:"type"`
	Value          int64          `json:"value"`
	MinOrderAmount int64          `json:"min_order_amount"`
	MaxDiscount    *int64         `json:"max_discount,omitempty"`
	MaxUses        *int           `json:"max_uses,omitempty"`
	UsedCount      int            `json:"used_count"`
	StartsAt       time.Time      `json:"starts_at"`
	ExpiresAt      time.Time      `json:"expires_at"`
	Active         bool           `json:"active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func toAdminPromotionResponse(p *promotion.Promotion) adminPromotionResponse {
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

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createPromotionRequest](w, r, h.validator)
	if !ok {
		return
	}

	promo, err := h.service.Create(r.Context(), req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminPromotionResponse(promo))
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := promotion.ListParams{
		OffsetPage: page,
	}

	promotions, total, err := h.service.List(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminPromotionResponse, len(promotions))
	for i, p := range promotions {
		out[i] = toAdminPromotionResponse(&p)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

type updatePromotionRequest struct {
	Code           string         `json:"code"             validate:"omitempty,min=1,max=50"`
	Type           promotion.Type `json:"type"             validate:"omitempty,oneof=percentage fixed_amount"`
	Value          *int64         `json:"value"            validate:"omitempty,min=1"`
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

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
