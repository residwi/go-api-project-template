package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// productResponse is shared by every endpoint that returns a product --
// public and admin alike expose the identical shape, so this is declared
// once here rather than duplicated per file.
//
// StockQuantity is Availability.OnHand, not Availability.Available: OnHand
// only reflects overall inventory depth (it moves on a restock or a manual
// adjustment), where Available moves on every order and would let a
// competitor infer order velocity per SKU -- the reservation count itself is
// never even computed onto product.Availability (see product/inventory.go).
// DeletedAt never appears: a soft-deleted product should not be
// distinguishable on the wire from one that simply 404s.
type productResponse struct {
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

type imageResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	URL       string    `json:"url"`
	AltText   *string   `json:"alt_text,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

func toProductResponse(p *product.Product) productResponse {
	images := make([]imageResponse, len(p.Images))
	for i, img := range p.Images {
		images[i] = imageResponse{
			ID:        img.ID,
			ProductID: img.ProductID,
			URL:       img.URL,
			AltText:   img.AltText,
			SortOrder: img.SortOrder,
			CreatedAt: img.CreatedAt,
		}
	}

	return productResponse{
		ID:             p.ID,
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		Price:          p.Price,
		CompareAtPrice: p.CompareAtPrice,
		Currency:       p.Currency,
		SKU:            p.SKU,
		Status:         p.Status,
		StockQuantity:  p.Availability.OnHand,
		Images:         images,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func (h *publicHandler) List(w http.ResponseWriter, r *http.Request) {
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

	response.Paginated(w, paging.NewCursorPageResult(out, nextCursor, hasMore))
}
