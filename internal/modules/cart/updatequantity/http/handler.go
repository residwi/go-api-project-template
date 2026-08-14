package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CartQuantityUpdater interface {
	Execute(ctx context.Context, userID, productID uuid.UUID, p updatequantity.Params) error
}

type Handler struct {
	cmd       CartQuantityUpdater
	validator *validator.Validator
}

func New(cmd CartQuantityUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("PUT /cart/items/{product_id}", h.Update)
}

type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

func (r updateQuantityRequest) toParams() updatequantity.Params {
	return updatequantity.Params{Quantity: r.Quantity}
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
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

	if err := h.cmd.Execute(r.Context(), uc.UserID, productID, req.toParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
