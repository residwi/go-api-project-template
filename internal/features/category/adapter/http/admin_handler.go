package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/category/domain"
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
	service CategoryManager
}

func NewAdminHandler(service CategoryManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) error {
	req, err := request.Bind[createCategoryRequest](w, r)
	if err != nil {
		return err
	}

	cat, err := h.service.Create(r.Context(), req.Name, req.Description, req.ParentID, req.SortOrder, req.Active)
	if err != nil {
		return err
	}

	response.Created(w, toAdminCategoryResponse(cat))
	return nil
}

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[updateCategoryRequest](w, r)
	if err != nil {
		return err
	}

	cat, err := h.service.Update(r.Context(), id, req.Name, req.Description, req.ParentID, req.SortOrder, req.Active)
	if err != nil {
		return err
	}

	response.OK(w, toAdminCategoryResponse(cat))
	return nil
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
