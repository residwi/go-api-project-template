package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/add"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CartAdder interface {
	Execute(ctx context.Context, userID uuid.UUID, p add.Params) error
}

type Handler struct {
	cmd       CartAdder
	validator *validator.Validator
}

func New(cmd CartAdder, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("POST /cart/items", h.Add)
}

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity"   validate:"required,min=1"`
}

func (r addItemRequest) toParams() add.Params {
	return add.Params{ProductID: r.ProductID, Quantity: r.Quantity}
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), uc.UserID, req.toParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}
