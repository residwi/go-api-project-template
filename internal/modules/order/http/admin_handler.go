package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *order.Service
	validator *validator.Validator
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := order.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
	}

	orders, total, err := h.service.AdminListAll(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]orderResponse, len(orders))
	for i, o := range orders {
		out[i] = toOrderResponse(&o)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.service.AdminGetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}

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
