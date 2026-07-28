package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/order"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// updateStatusRequest has no params.go counterpart: order.Service.
// AdminUpdateStatus already takes a plain order.Status, not a request
// struct, so there is no dto-in-the-core cycle to break here.
type updateStatusRequest struct {
	Status string `json:"status" validate:"required"`
}

func (h *adminHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateStatusRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.AdminUpdateStatus(r.Context(), id, order.Status(req.Status)); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
