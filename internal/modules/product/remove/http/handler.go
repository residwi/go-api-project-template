package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ProductDeleter interface {
	Execute(ctx context.Context, id uuid.UUID) error
}

type Handler struct {
	cmd ProductDeleter
}

func New(cmd ProductDeleter) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("DELETE /products/{id}", h.delete)
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
