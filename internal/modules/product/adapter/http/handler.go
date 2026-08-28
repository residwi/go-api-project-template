package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/response"
)

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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	cursor := paging.ParseCursorPage(r)

	params := product.PublishedListParams{
		Cursor: cursor.Cursor,
		Limit:  cursor.Limit,
		Search: r.URL.Query().Get("search"),
	}

	if catID := r.URL.Query().Get("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err != nil {
			response.BadRequest(w, "invalid category_id")
			return
		}
		params.CategoryID = &id
	}
	if minStr := r.URL.Query().Get("min_price"); minStr != "" {
		v, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			response.BadRequest(w, "invalid min_price")
			return
		}
		params.MinPrice = &v
	}
	if maxStr := r.URL.Query().Get("max_price"); maxStr != "" {
		v, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			response.BadRequest(w, "invalid max_price")
			return
		}
		params.MaxPrice = &v
	}

	products, nextCursor, hasMore, err := h.service.ListPublished(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]productResponse, len(products))
	for i, p := range products {
		out[i] = toProductResponse(&p)
	}

	response.OK(w, paging.NewCursorPageResult(out, nextCursor, hasMore))
}

func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		response.BadRequest(w, "slug is required")
		return
	}

	p, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toProductResponse(p))
}
