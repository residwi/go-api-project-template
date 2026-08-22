package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ReviewDeleter interface {
	Delete(ctx context.Context, id uuid.UUID) error
}

type AdminHandler struct {
	service ReviewDeleter
}

func NewAdminHandler(service ReviewDeleter) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
