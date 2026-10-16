package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
