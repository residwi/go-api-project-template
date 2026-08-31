package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

type PromotionApplier interface {
	Apply(ctx context.Context, code string, orderAmount int64) (int64, error)
}

type Handler struct {
	service   PromotionApplier
	validator *validator.Validator
}

func NewHandler(service PromotionApplier, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := request.Bind[applyRequest](w, r, h.validator)
	if !ok {
		return
	}

	discount, err := h.service.Apply(r.Context(), req.Code, req.Subtotal)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toApplyResponse(req.Code, discount))
}
