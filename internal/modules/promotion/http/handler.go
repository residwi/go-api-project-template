package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *promotion.Service
	validator *validator.Validator
}

type applyRequest struct {
	Code     string `json:"code"     validate:"required"`
	Subtotal int64  `json:"subtotal" validate:"required,min=1"`
}

// The computed discount only: usage counters and per-user limits are the store's
// business, not a shopper's.
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

func (h *handler) Apply(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[applyRequest](w, r, h.validator)
	if !ok {
		return
	}

	discount, err := h.service.Validate(r.Context(), req.Code, req.Subtotal)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toApplyResponse(req.Code, discount))
}
