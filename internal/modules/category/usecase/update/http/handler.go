package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/update"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CategoryUpdater interface {
	Execute(ctx context.Context, id uuid.UUID, p update.Params) (*domain.Category, error)
}

type Handler struct {
	cmd       CategoryUpdater
	validator *validator.Validator
}

func New(cmd CategoryUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

type categoryResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	SortOrder   int        `json:"sort_order"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func toCategoryResponse(c *domain.Category) categoryResponse {
	return categoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		ParentID:    c.ParentID,
		SortOrder:   c.SortOrder,
		Active:      c.Active,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

type updateCategoryRequest struct {
	Name        *string    `json:"name"        validate:"omitempty,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id"   validate:"omitempty"`
	SortOrder   *int       `json:"sort_order"  validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

func (r updateCategoryRequest) toParams() update.Params {
	return update.Params{
		Name:        r.Name,
		Description: r.Description,
		ParentID:    r.ParentID,
		SortOrder:   r.SortOrder,
		Active:      r.Active,
	}
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.cmd.Execute(r.Context(), id, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toCategoryResponse(cat))
}
