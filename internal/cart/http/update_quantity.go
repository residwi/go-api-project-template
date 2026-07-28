package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/cart"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// updateQuantityRequest carries the validation rules, moved here verbatim
// from the old cart.UpdateItemRequest.
type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

// toUpdateQuantityParams is the seam: HTTP's validation vocabulary stops
// here, and the service receives a plain input struct.
func (r updateQuantityRequest) toUpdateQuantityParams() cart.UpdateQuantityParams {
	return cart.UpdateQuantityParams{Quantity: r.Quantity}
}

func (h *handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateQuantityRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req.toUpdateQuantityParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
