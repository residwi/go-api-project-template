package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// NotificationMarker is what Handler needs from markread.Command:
// markread.Command satisfies it directly, so nothing sits between them, and
// the mockery-generated mock is the other implementation, used in
// handler_test.go.
type NotificationMarker interface {
	Execute(ctx context.Context, userID, id uuid.UUID) error
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd NotificationMarker
}

func New(cmd NotificationMarker) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("PUT /notifications/{id}/read", h.MarkRead)
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
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
