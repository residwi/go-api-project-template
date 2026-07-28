package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	"github.com/residwi/go-api-project-template/internal/user"
)

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), user.DeleteParams{
		RequesterID: uc.UserID,
		TargetID:    id,
	}); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
