package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/features/promotion/domain"
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

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) error {
	req, err := request.Bind[createPromotionRequest](w, r)
	if err != nil {
		return err
	}

	promo, err := h.service.Create(
		r.Context(), req.Code, req.Type, req.Value, req.MinOrderAmount,
		req.MaxDiscount, req.MaxUses, req.StartsAt, req.ExpiresAt, req.Active,
	)
	if err != nil {
		return err
	}

	response.Created(w, toAdminPromotionResponse(promo))
	return nil
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) error {
	page := paging.ParseOffsetPage(r)
	params := promotion.AdminListParams{OffsetPage: page}

	promotions, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		return err
	}

	out := make([]adminPromotionResponse, len(promotions))
	for i, p := range promotions {
		out[i] = toAdminPromotionResponse(&p)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
	return nil
}

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[updatePromotionRequest](w, r)
	if err != nil {
		return err
	}

	promo, err := h.service.Update(
		r.Context(), id, req.Code, req.Type, req.Value, req.MinOrderAmount,
		req.MaxDiscount, req.MaxUses, req.StartsAt, req.ExpiresAt, req.Active,
	)
	if err != nil {
		return err
	}

	response.OK(w, toAdminPromotionResponse(promo))
	return nil
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
