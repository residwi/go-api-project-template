package promotion

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

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	promo, err := h.service.Create(r.Context(), req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, promo)
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := core.ParseOffsetPage(r)
	params := ListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
	}

	promotions, total, err := h.service.List(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Paginated(w, core.NewOffsetPageResult(promotions, page, total))
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req UpdateRequest
	if decodeErr := response.DecodeJSON(w, r, &req); decodeErr != nil {
		response.HandleErr(w, decodeErr)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	promo, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, promo)
}

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
