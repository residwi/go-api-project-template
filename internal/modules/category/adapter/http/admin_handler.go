package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type CategoryManager interface {
	Create(
		ctx context.Context,
		name string,
		description *string,
		parentID *uuid.UUID,
		sortOrder *int,
		active *bool,
	) (*domain.Category, error)
	Update(
		ctx context.Context,
		id uuid.UUID,
		name *string,
		description *string,
		parentID *uuid.UUID,
		sortOrder *int,
		active *bool,
	) (*domain.Category, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AdminHandler struct {
	service   CategoryManager
	validator *validator.Validator
}

func NewAdminHandler(service CategoryManager, v *validator.Validator) *AdminHandler {
	return &AdminHandler{service: service, validator: v}
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := request.Bind[createCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Create(r.Context(), req.Name, req.Description, req.ParentID, req.SortOrder, req.Active)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminCategoryResponse(cat))
}

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[updateCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Update(r.Context(), id, req.Name, req.Description, req.ParentID, req.SortOrder, req.Active)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminCategoryResponse(cat))
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
