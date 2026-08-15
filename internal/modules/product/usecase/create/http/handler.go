package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/create"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ProductCreator interface {
	Execute(ctx context.Context, p create.Params) (*domain.Product, error)
}

type Handler struct {
	usecase   ProductCreator
	validator *validator.Validator
}

func New(usecase ProductCreator, v *validator.Validator) *Handler {
	return &Handler{usecase: usecase, validator: v}
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
		SKU:            p.SKU,
		Status:         p.Status,
		StockQuantity:  p.Availability.OnHand,
		Images:         toImageResponses(p.Images),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type createProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           string     `json:"name"             validate:"required,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          int64      `json:"price"            validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         string     `json:"status"           validate:"omitempty,oneof=draft published archived"`
}

func (r createProductRequest) toParams() create.Params {
	p := create.Params{
		CategoryID:  r.CategoryID,
		Name:        r.Name,
		Description: r.Description,
		Price:       money.New(r.Price, r.Currency),
		SKU:         r.SKU,
		Status:      r.Status,
	}
	if r.CompareAtPrice != nil {
		compareAt := money.New(*r.CompareAtPrice, r.Currency)
		p.CompareAtPrice = &compareAt
	}
	return p
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.usecase.Execute(r.Context(), req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toProductResponse(p))
}
