package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// CategoryReader is what Handler needs from query.Reader: query.Reader
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type CategoryReader interface {
	List(ctx context.Context) ([]domain.Category, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Category, error)
}

type Handler struct {
	reader CategoryReader
}

func New(reader CategoryReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(api *middleware.RouteGroup) {
	api.HandleFunc("GET /categories", h.list)
	api.HandleFunc("GET /categories/{slug}", h.getBySlug)
}

// The public shape. Omitting Active and SortOrder is what matters: these
// endpoints are unauthenticated and List has no WHERE active filter, so
// naming them would let anyone enumerate unpublished categories. Admin
// endpoints get the fuller response create and update return.
type categoryResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
}

func toCategoryResponse(c *domain.Category) categoryResponse {
	return categoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		ParentID:    c.ParentID,
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	categories, err := h.reader.List(r.Context())
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

func (h *Handler) getBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		response.BadRequest(w, "slug is required")
		return
	}

	cat, err := h.reader.GetBySlug(r.Context(), slug)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toCategoryResponse(cat))
}
