package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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
	service PromotionManager
}

func NewAdminHandler(service PromotionManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := request.Bind[createPromotionRequest](w, r)
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

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[updatePromotionRequest](w, r)
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
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
