package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// CartClearer is what Handler needs from empty.Command: empty.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in
// handler_test.go.
type CartClearer interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd CartClearer
}

func New(cmd CartClearer) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("DELETE /cart", h.clear)
}

func (h *Handler) clear(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.cmd.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
