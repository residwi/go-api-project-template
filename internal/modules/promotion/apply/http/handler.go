package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PromotionApplier interface {
	Execute(ctx context.Context, code string, orderAmount int64) (int64, error)
}

type Handler struct {
	cmd       PromotionApplier
	validator *validator.Validator
}

func New(cmd PromotionApplier, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("POST /promotions/apply", h.apply)
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

func (h *Handler) apply(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[applyRequest](w, r, h.validator)
	if !ok {
		return
	}

	discount, err := h.cmd.Execute(r.Context(), req.Code, req.Subtotal)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toApplyResponse(req.Code, discount))
}
