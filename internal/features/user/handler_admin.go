package user

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type adminHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := core.ParseOffsetPage(r)
	params := ListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Role:     r.URL.Query().Get("role"),
		Search:   r.URL.Query().Get("search"),
	}

	if activeStr := r.URL.Query().Get("active"); activeStr != "" {
		active := activeStr == "true"
		params.Active = &active
	}

	users, total, err := h.service.List(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Paginated(w, core.NewOffsetPageResult(users, page, total))
}

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	user, err := h.service.AdminGetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, user)
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[AdminUpdateUserRequest](w, r, h.validator)
	if !ok {
		return
	}

	user, err := h.service.AdminUpdate(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, user)
}

func (h *adminHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[UpdateRoleRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateRole(r.Context(), uc.UserID, id, req.Role); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
