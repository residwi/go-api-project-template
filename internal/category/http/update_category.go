package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type updateCategoryRequest struct {
	Name        *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id" validate:"omitempty"`
	SortOrder   *int       `json:"sort_order" validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

func (r updateCategoryRequest) toUpdateParams() category.UpdateParams {
	return category.UpdateParams{
		Name:        r.Name,
		Description: r.Description,
		ParentID:    r.ParentID,
		SortOrder:   r.SortOrder,
		Active:      r.Active,
	}
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.service.Update(r.Context(), id, req.toUpdateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toCategoryResponse(cat))
}
