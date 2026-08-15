package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CartClearer interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}

type Handler struct {
	usecase CartClearer
}

func New(usecase CartClearer) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.usecase.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
