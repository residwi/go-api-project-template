package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type AllNotificationsMarker interface {
	Execute(ctx context.Context, userID uuid.UUID) error
}

type Handler struct {
	usecase AllNotificationsMarker
}

func New(usecase AllNotificationsMarker) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.usecase.Execute(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
