package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PromotionDeleter interface {
	Execute(ctx context.Context, id uuid.UUID) error
}

type Handler struct {
	cmd PromotionDeleter
}

func New(cmd PromotionDeleter) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
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
