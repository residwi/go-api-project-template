package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/create"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ProductCreator is what Handler needs from create.Command: create.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type ProductCreator interface {
	Execute(ctx context.Context, p create.Params) (*domain.Product, error)
}

type Handler struct {
	cmd       ProductCreator
	validator *validator.Validator
}

func New(cmd ProductCreator, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("POST /products", h.create)
}

// Declared here, not shared with product's other slices. Each endpoint holds
// its own copy so one endpoint's new field cannot appear in another's
// response. The caller is always an admin route, so this keeps SKU and Status,
// which the public productResponse in query/http drops.
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

// *int64, not money.Money: a struct is never empty to encoding/json, so an
// `omitempty` money key would appear as 0 on every product that should omit it.
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

// An empty `currency` is passed through: the default is a business rule
// create.Execute owns.
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

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.cmd.Execute(r.Context(), req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toProductResponse(p))
}
