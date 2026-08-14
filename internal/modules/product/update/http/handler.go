package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/update"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ProductUpdater interface {
	Execute(ctx context.Context, id uuid.UUID, p update.Params) (*domain.Product, error)
}

type Handler struct {
	cmd       ProductUpdater
	validator *validator.Validator
}

func New(cmd ProductUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
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

type updateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           *string    `json:"name"             validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          *int64     `json:"price"            validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         *string    `json:"status"           validate:"omitempty,oneof=draft published archived"`
}

func (r updateProductRequest) toParams() (update.Params, error) {
	p := update.Params{
		CategoryID:  r.CategoryID,
		Name:        r.Name,
		Description: r.Description,
		SKU:         r.SKU,
		Status:      r.Status,
	}

	switch {
	case r.Price == nil && r.Currency == nil && r.CompareAtPrice == nil:
		return p, nil
	case r.Price == nil || r.Currency == nil:
		return update.Params{}, fmt.Errorf(
			"%w: price and currency must be supplied together, and compare_at_price requires both",
			apperror.ErrBadRequest)
	}

	price := money.New(*r.Price, *r.Currency)
	p.Price = &price
	if r.CompareAtPrice != nil {
		compareAt := money.New(*r.CompareAtPrice, *r.Currency)
		p.CompareAtPrice = &compareAt
	}
	return p, nil
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	params, err := req.toParams()
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	p, err := h.cmd.Execute(r.Context(), id, params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toProductResponse(p))
}
