package product

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type publicHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *publicHandler) List(w http.ResponseWriter, r *http.Request) {
	cursor := core.ParseCursorPage(r)

	params := PublishedListParams{
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

	response.Paginated(w, core.NewCursorPageResult(products, nextCursor, hasMore))
}

func (h *publicHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
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

	response.OK(w, p)
}
