package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type createCategoryRequest struct {
	Name        string     `json:"name" validate:"required,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id" validate:"omitempty"`
	SortOrder   *int       `json:"sort_order" validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

func (r createCategoryRequest) toCreateParams() category.CreateParams {
	return category.CreateParams{
		Name:        r.Name,
		Description: r.Description,
		ParentID:    r.ParentID,
		SortOrder:   r.SortOrder,
		Active:      r.Active,
	}
}

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Create(r.Context(), req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toCategoryResponse(cat))
}
