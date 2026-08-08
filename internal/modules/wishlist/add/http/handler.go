package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/add"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ItemAdder is what Handler needs from add.Command: add.Command satisfies it
// directly, so nothing sits between them, and the mockery-generated mock is
// the other implementation, used in handler_test.go.
type ItemAdder interface {
	Execute(ctx context.Context, userID uuid.UUID, p add.Params) error
}

type Handler struct {
	cmd       ItemAdder
	validator *validator.Validator
}

func New(cmd ItemAdder, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("POST /wishlist/items", h.add)
}

// addItemRequest carries the validation rules. They live here, not in the
// core: a service called from a worker should not inherit HTTP's validation
// vocabulary.
type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
}

func (r addItemRequest) toParams() add.Params {
	return add.Params{ProductID: r.ProductID}
}

func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
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
