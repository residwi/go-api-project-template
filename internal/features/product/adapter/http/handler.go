package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/features/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

const maxSlugLen = 255 // products.slug column width; a longer slug cannot exist

type ProductReader interface {
	ListPublished(ctx context.Context, params product.PublishedListParams) ([]domain.Product, string, bool, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Product, error)
}

type Handler struct {
	service ProductReader
}

func NewHandler(service ProductReader) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListPublished(w http.ResponseWriter, r *http.Request) error {
	cursor := paging.ParseCursorPage(r)

	params := product.PublishedListParams{
		Cursor: cursor.Cursor,
		Limit:  cursor.Limit,
		Search: r.URL.Query().Get("search"),
	}

	if catID := r.URL.Query().Get("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err != nil {
			return fmt.Errorf("%w: invalid category_id", errs.ErrBadRequest)
		}
		params.CategoryID = &id
	}
	if minStr := r.URL.Query().Get("min_price"); minStr != "" {
		v, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			return fmt.Errorf("%w: invalid min_price", errs.ErrBadRequest)
		}
		params.MinPrice = &v
	}
	if maxStr := r.URL.Query().Get("max_price"); maxStr != "" {
		v, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			return fmt.Errorf("%w: invalid max_price", errs.ErrBadRequest)
		}
		params.MaxPrice = &v
	}

	products, nextCursor, hasMore, err := h.service.ListPublished(r.Context(), params)
	if err != nil {
		return err
	}

	out := make([]productResponse, len(products))
	for i, p := range products {
		out[i] = toProductResponse(&p)
	}

	response.OK(w, paging.NewCursorPageResult(out, nextCursor, hasMore))
	return nil
}

func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	if slug == "" {
		return fmt.Errorf("%w: slug is required", errs.ErrBadRequest)
	}
	if len(slug) > maxSlugLen {
		return errs.ErrNotFound
	}

	p, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		return err
	}

	response.OK(w, toProductResponse(p))
	return nil
}
