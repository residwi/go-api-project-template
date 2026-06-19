package product

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type adminHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[CreateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.service.Create(r.Context(), req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, p)
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := core.ParseOffsetPage(r)

	params := AdminListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
	}

	if catID := r.URL.Query().Get("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err != nil {
			response.BadRequest(w, "invalid category_id")
			return
		}
		params.CategoryID = &id
	}

	products, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Paginated(w, core.NewOffsetPageResult(products, page, total))
}

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

	response.OK(w, p)
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[UpdateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, p)
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
