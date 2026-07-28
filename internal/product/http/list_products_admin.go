package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)

	params := product.AdminListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
	}

	if catID := r.URL.Query().Get("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err != nil {
			response.BadRequest(w, "invalid category_id")
			return
		}
		params.CategoryID = &id
	}

	products, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]productResponse, len(products))
	for i, p := range products {
		out[i] = toProductResponse(&p)
	}

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
}
