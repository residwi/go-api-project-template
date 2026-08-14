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

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
}

func (r addItemRequest) toParams() add.Params {
	return add.Params{ProductID: r.ProductID}
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
