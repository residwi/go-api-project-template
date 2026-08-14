package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CartItemRemover interface {
	Execute(ctx context.Context, userID, productID uuid.UUID) error
}

type Handler struct {
	cmd CartItemRemover
}

func New(cmd CartItemRemover) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("DELETE /cart/items/{product_id}", h.Remove)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
