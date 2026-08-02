package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type publicHandler struct {
	service   *category.Service
	validator *validator.Validator
}

// categoryResponse is the public wire contract, shared by the public list
// and get-by-slug endpoints. It deliberately omits SortOrder, Active, and
// the audit timestamps: those are merchandising/moderation details for admin
// tooling, not something an anonymous shopper needs. Active matters most --
// GET /categories and GET /categories/{slug} sit on the unauthenticated
// route group with no WHERE active filter in the repository (see
// category/postgres/repository.go's List), so naming Active here would let
// an anonymous caller enumerate staged/unpublished categories. Admin
// mutation endpoints get the fuller adminCategoryResponse (see
// admin_handler.go) with every field intact.
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

func (h *publicHandler) List(w http.ResponseWriter, r *http.Request) {
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

func (h *publicHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
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
