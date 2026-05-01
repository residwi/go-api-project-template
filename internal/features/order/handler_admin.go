package order

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type adminHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := core.ParseOffsetPage(r)
	params := AdminListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Status:   r.URL.Query().Get("status"),
	}

	orders, total, err := h.service.AdminListAll(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Paginated(w, core.NewOffsetPageResult(orders, page, total))
}

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	order, err := h.service.AdminGetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, order)
}

func (h *adminHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req AdminUpdateStatusRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	if err := h.service.AdminUpdateStatus(r.Context(), id, Status(req.Status)); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
