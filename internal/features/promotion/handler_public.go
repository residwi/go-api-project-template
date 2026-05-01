package promotion

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type publicHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *publicHandler) Apply(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	var req ApplyRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	discount, err := h.service.Validate(r.Context(), req.Code, req.Subtotal)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, ApplyResponse{
		Code:     req.Code,
		Discount: discount,
	})
}
