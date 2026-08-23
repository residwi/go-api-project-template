package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
	"github.com/residwi/go-api-project-template/internal/server/response"
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

type applyRequest struct {
	Code     string `json:"code"     validate:"required"`
	Subtotal int64  `json:"subtotal" validate:"required,min=1"`
}

type applyResponse struct {
	Code     string `json:"code"`
	Discount int64  `json:"discount"`
}

func toApplyResponse(code string, discount int64) applyResponse {
	return applyResponse{
		Code:     code,
		Discount: discount,
	}
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[applyRequest](w, r, h.validator)
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
