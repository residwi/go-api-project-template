package category

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type adminHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[CreateCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Create(r.Context(), req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, cat)
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[UpdateCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, cat)
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
