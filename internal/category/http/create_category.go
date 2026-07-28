package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// adminCategoryResponse is the admin wire contract -- unlike the public
// categoryResponse (list_categories.go), it keeps SortOrder, Active, and the
// audit timestamps: an operator needs to see a category's moderation state
// and merchandising order to manage it. Used by every admin endpoint that
// returns a category body: this file's Create and update_category.go's
// Update.
type adminCategoryResponse struct {
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

func toAdminCategoryResponse(c *category.Category) adminCategoryResponse {
	return adminCategoryResponse{
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

	response.Created(w, toAdminCategoryResponse(cat))
}
