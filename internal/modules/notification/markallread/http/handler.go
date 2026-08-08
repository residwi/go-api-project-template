package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// AllNotificationsMarker is what Handler needs from markallread.Command:
// markallread.Command satisfies it directly, so nothing sits between them,
// and the mockery-generated mock is the other implementation, used in
// handler_test.go.
type AllNotificationsMarker interface {
	Execute(ctx context.Context, userID uuid.UUID) error
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd AllNotificationsMarker
}

func New(cmd AllNotificationsMarker) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("PUT /notifications/read-all", h.MarkAllRead)
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
