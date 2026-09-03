package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type PromotionApplier interface {
	Apply(ctx context.Context, code string, orderAmount int64) (int64, error)
}

type Handler struct {
	service PromotionApplier
}

func NewHandler(service PromotionApplier) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	_, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := request.Bind[applyRequest](w, r)
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
