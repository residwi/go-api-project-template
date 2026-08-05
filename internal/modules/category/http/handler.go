package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *category.Service
	validator *validator.Validator
}

// The public shape. Omitting Active is what matters: these endpoints are
// unauthenticated and the repository's List has no WHERE active filter, so
// naming it would let anyone enumerate unpublished categories. Admin endpoints
// get the fuller adminCategoryResponse.
type categoryResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
}

func toCategoryResponse(c *category.Category) categoryResponse {
	return categoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		ParentID:    c.ParentID,
	}
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.List(r.Context())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]categoryResponse, len(categories))
	for i, c := range categories {
		out[i] = toCategoryResponse(&c)
	}

	response.OK(w, out)
}

func (h *handler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		response.BadRequest(w, "slug is required")
		return
	}

	cat, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toCategoryResponse(cat))
}
