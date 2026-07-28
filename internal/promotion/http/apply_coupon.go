package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// applyRequest has no params.go counterpart: promotion.Service.Validate
// already takes plain (code, orderAmount) arguments, not a request struct,
// so there is no dto-in-the-core cycle to break here.
type applyRequest struct {
	Code     string `json:"code" validate:"required"`
	Subtotal int64  `json:"subtotal" validate:"required,min=1"`
}

// applyResponse returns only the computed discount, never the promotion's
// internal usage counters (UsedCount, MaxUses) or per-user limits
// (MinOrderAmount, MaxDiscount) -- those are the store's business, not a
// shopper's.
type applyResponse struct {
	Code     string `json:"code"`
	Discount int64  `json:"discount"`
}

func (h *publicHandler) Apply(w http.ResponseWriter, r *http.Request) {
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

	response.OK(w, applyResponse{
		Code:     req.Code,
		Discount: discount,
	})
}
