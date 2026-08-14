package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/update"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PromotionUpdater interface {
	Execute(ctx context.Context, id uuid.UUID, p update.Params) (*domain.Promotion, error)
}

type Handler struct {
	cmd       PromotionUpdater
	validator *validator.Validator
}

func New(cmd PromotionUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
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

func (r updatePromotionRequest) toParams() update.Params {
	return update.Params{
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

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updatePromotionRequest](w, r, h.validator)
	if !ok {
		return
	}

	promo, err := h.cmd.Execute(r.Context(), id, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminPromotionResponse(promo))
}
