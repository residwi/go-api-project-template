package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ItemRemover is what Handler needs from remove.Command: remove.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in
// handler_test.go.
type ItemRemover interface {
	Execute(ctx context.Context, userID, productID uuid.UUID) error
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd ItemRemover
}

func New(cmd ItemRemover) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", h.remove)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
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
