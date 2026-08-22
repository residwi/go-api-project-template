package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PromotionManager interface {
	Create(
		ctx context.Context,
		code string,
		promoType domain.Type,
		value int64,
		minOrderAmount int64,
		maxDiscount *int64,
		maxUses *int,
		startsAt time.Time,
		expiresAt time.Time,
		active bool,
	) (*domain.Promotion, error)
	ListAdmin(ctx context.Context, params promotion.AdminListParams) ([]domain.Promotion, int, error)
	Update(
		ctx context.Context,
		id uuid.UUID,
		code string,
		promoType domain.Type,
		value *int64,
		minOrderAmount *int64,
		maxDiscount *int64,
		maxUses *int,
		startsAt *time.Time,
		expiresAt *time.Time,
		active *bool,
	) (*domain.Promotion, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AdminHandler struct {
	service   PromotionManager
	validator *validator.Validator
}

func NewAdminHandler(service PromotionManager, v *validator.Validator) *AdminHandler {
	return &AdminHandler{service: service, validator: v}
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

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createPromotionRequest](w, r, h.validator)
	if !ok {
		return
	}

	promo, err := h.service.Create(
		r.Context(), req.Code, req.Type, req.Value, req.MinOrderAmount,
		req.MaxDiscount, req.MaxUses, req.StartsAt, req.ExpiresAt, req.Active,
	)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminPromotionResponse(promo))
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := promotion.AdminListParams{OffsetPage: page}

	promotions, total, err := h.service.ListAdmin(r.Context(), params)
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

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updatePromotionRequest](w, r, h.validator)
	if !ok {
		return
	}

	promo, err := h.service.Update(
		r.Context(), id, req.Code, req.Type, req.Value, req.MinOrderAmount,
		req.MaxDiscount, req.MaxUses, req.StartsAt, req.ExpiresAt, req.Active,
	)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminPromotionResponse(promo))
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
