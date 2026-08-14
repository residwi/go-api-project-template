package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type UseCase interface {
	Execute(ctx context.Context, userID, orderID uuid.UUID) error
}

type Handler struct {
	cmd UseCase
}

func New(cmd UseCase) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
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
