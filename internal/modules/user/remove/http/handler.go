package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/user/remove"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type UserDeleter interface {
	Execute(ctx context.Context, p remove.Params) error
}

type Handler struct {
	cmd UserDeleter
}

func New(cmd UserDeleter) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), remove.Params{
		RequesterID: uc.UserID,
		TargetID:    id,
	}); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
