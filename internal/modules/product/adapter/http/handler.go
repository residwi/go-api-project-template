package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
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

func compareAtPriceAmount(m *money.Money) *int64 {
	if m == nil {
		return nil
	}
	return &m.Amount
}

func toImageResponses(images []domain.Image) []imageResponse {
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

func toProductResponse(p *domain.Product) productResponse {
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
