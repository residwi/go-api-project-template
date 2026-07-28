package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// adminProductResponse is the admin wire contract -- unlike the public
// productResponse (list_products.go), it keeps SKU and Status: an operator
// needs a SKU to reconcile inventory and needs to see draft/archived
// products distinctly from published ones. Used by every admin endpoint
// that returns a product body: this file's List, get_product_admin.go's
// Get, create_product.go's Create, and update_product.go's Update.
type adminProductResponse struct {
	ID             uuid.UUID       `json:"id"`
	CategoryID     *uuid.UUID      `json:"category_id,omitempty"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    *string         `json:"description,omitempty"`
	Price          int64           `json:"price"`
	CompareAtPrice *int64          `json:"compare_at_price,omitempty"`
	Currency       string          `json:"currency"`
	SKU            *string         `json:"sku,omitempty"`
	Status         string          `json:"status"`
	StockQuantity  int             `json:"stock_quantity"`
	Images         []imageResponse `json:"images,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func toAdminProductResponse(p *product.Product) adminProductResponse {
	return adminProductResponse{
		ID:             p.ID,
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		Price:          p.Price.Amount,
		CompareAtPrice: compareAtPriceAmount(p.CompareAtPrice),
		Currency:       p.Price.Currency,
		SKU:            p.SKU,
		Status:         p.Status,
		StockQuantity:  p.Availability.OnHand,
		Images:         toImageResponses(p.Images),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

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

	out := make([]adminProductResponse, len(products))
	for i, p := range products {
		out[i] = toAdminProductResponse(&p)
	}

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
}
