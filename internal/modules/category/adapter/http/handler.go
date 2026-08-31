package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type CategoryReader interface {
	List(ctx context.Context) ([]domain.Category, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Category, error)
}

type Handler struct {
	service CategoryReader
}

func NewHandler(service CategoryReader) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) {
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
