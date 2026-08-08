package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// CategoryDeleter is what Handler needs from remove.Command: remove.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in
// handler_test.go.
type CategoryDeleter interface {
	Execute(ctx context.Context, id uuid.UUID) error
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd CategoryDeleter
}

func New(cmd CategoryDeleter) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("DELETE /categories/{id}", h.delete)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
