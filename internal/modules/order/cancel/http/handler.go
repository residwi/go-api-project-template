package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// Command is what Handler needs from cancel.Command: cancel.Command satisfies
// it directly, so nothing sits between them, and the mockery-generated mock
// is the other implementation, used in handler_test.go.
type Command interface {
	Execute(ctx context.Context, userID, orderID uuid.UUID) error
}

type Handler struct {
	cmd Command
}

func New(cmd Command) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("POST /orders/{id}/cancel", h.cancel)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
