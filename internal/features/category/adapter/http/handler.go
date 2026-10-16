package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/features/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	categories, err := h.service.List(r.Context())
	if err != nil {
		return err
	}

	out := make([]categoryResponse, len(categories))
	for i, c := range categories {
		out[i] = toCategoryResponse(&c)
	}

	response.OK(w, out)
	return nil
}

func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	if slug == "" {
		return fmt.Errorf("%w: slug is required", errs.ErrBadRequest)
	}

	cat, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		return err
	}

	response.OK(w, toCategoryResponse(cat))
	return nil
}
