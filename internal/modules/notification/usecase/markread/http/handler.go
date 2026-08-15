package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type NotificationMarker interface {
	Execute(ctx context.Context, userID, id uuid.UUID) error
}

type Handler struct {
	usecase NotificationMarker
}

func New(usecase NotificationMarker) *Handler {
	return &Handler{usecase: usecase}
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

	if err := h.usecase.Execute(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
