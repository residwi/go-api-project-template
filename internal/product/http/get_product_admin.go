package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	p, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toProductResponse(p))
}
