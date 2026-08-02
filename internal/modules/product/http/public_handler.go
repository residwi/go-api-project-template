package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type publicHandler struct {
	service   *product.Service
	validator *validator.Validator
}

// productResponse is this endpoint's public wire contract, shared by the
// public list and get-by-slug endpoints. SKU is dropped: it is a
// merchandising/inventory detail a shopper has no use for. Status is
// dropped for the same reason review deliberately dropped its own Status --
// every product List can return is already product.StatusPublished
// (ListPublished filters WHERE status = 'published'), so on that path the
// field would be a constant, not information. Admin endpoints get the
// fuller adminProductResponse (see admin_handler.go), which keeps both.
//
// StockQuantity is Availability.OnHand, not Availability.Available: OnHand
// only reflects overall inventory depth (it moves on a restock or a manual
// adjustment), where Available moves on every order and would let a
// competitor infer order velocity per SKU -- the reservation count itself is
// never even computed onto product.Availability (see product/inventory.go).
// DeletedAt never appears: a soft-deleted product should not be
// distinguishable on the wire from one that simply 404s.
//
// Price and CompareAtPrice are int64 minor units even though product.Product
// now holds them as money.Money, and `currency` is emitted once for both:
// flattening each value here -- rather than letting the type marshal itself --
// is what keeps that shape the adapter's decision. See internal/money/doc.go.
type productResponse struct {
	ID             uuid.UUID       `json:"id"`
	CategoryID     *uuid.UUID      `json:"category_id,omitempty"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    *string         `json:"description,omitempty"`
	Price          int64           `json:"price"`
	CompareAtPrice *int64          `json:"compare_at_price,omitempty"`
	Currency       string          `json:"currency"`
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

// compareAtPriceAmount flattens an optional compare-at price to its amount.
// Shared by the public and admin mappers (admin_handler.go).
//
// The return type stays *int64, not money.Money, because `compare_at_price` is
// `omitempty` and has always been absent from the body when a product has no
// compare-at price. A money.Money would be a struct -- never empty as far as
// encoding/json is concerned -- so the key would appear as 0 on every product
// that used to omit it, which is a wire change dressed up as a type change.
// The currency is not repeated either: it is the product's, published once
// under `currency`.
func compareAtPriceAmount(m *money.Money) *int64 {
	if m == nil {
		return nil
	}
	return &m.Amount
}

// toImageResponses maps a product's images onto the wire shape. Shared by
// the public and admin product responses (admin_handler.go) -- an
// image carries no field that needs hiding from either audience.
func toImageResponses(images []product.Image) []imageResponse {
	out := make([]imageResponse, len(images))
	for i, img := range images {
		out[i] = imageResponse{
			ID:        img.ID,
			ProductID: img.ProductID,
			URL:       img.URL,
			AltText:   img.AltText,
			SortOrder: img.SortOrder,
			CreatedAt: img.CreatedAt,
		}
	}
	return out
}

func toProductResponse(p *product.Product) productResponse {
	return productResponse{
		ID:             p.ID,
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		Price:          p.Price.Amount,
		CompareAtPrice: compareAtPriceAmount(p.CompareAtPrice),
		Currency:       p.Price.Currency,
		StockQuantity:  p.Availability.OnHand,
		Images:         toImageResponses(p.Images),
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

	response.OK(w, toProductResponse(p))
}
