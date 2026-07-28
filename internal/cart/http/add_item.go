package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/cart"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// addItemRequest carries the validation rules, moved here verbatim from the
// old cart.AddItemRequest. They live in the transport, not the core: a
// service reachable from a worker should not inherit HTTP's validation
// vocabulary.
type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity" validate:"required,min=1"`
}

// toAddItemParams is the seam: HTTP's validation vocabulary stops here, and
// the service receives a plain input struct.
func (r addItemRequest) toAddItemParams() cart.AddItemParams {
	return cart.AddItemParams{ProductID: r.ProductID, Quantity: r.Quantity}
}

func (h *handler) AddItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.AddItem(r.Context(), uc.UserID, req.toAddItemParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}
