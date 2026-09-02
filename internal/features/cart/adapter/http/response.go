package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
)

type cartResponse struct {
	ID    uuid.UUID          `json:"id"`
	Items []cartItemResponse `json:"items"`
	Total int64              `json:"total"`
}

type cartItemResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	Currency  string    `json:"currency"`
	Quantity  int       `json:"quantity"`
	Available int       `json:"available_stock"`
	Sellable  bool      `json:"sellable"`
	CreatedAt time.Time `json:"created_at"`
}

func toCartResponse(c *domain.Cart) (cartResponse, error) {
	out := cartResponse{ID: c.ID, Items: make([]cartItemResponse, len(c.Items))}
	for i, it := range c.Items {
		out.Items[i] = cartItemResponse{
			ID:        it.ID,
			ProductID: it.ProductID,
			Name:      it.Product.Name,
			Price:     it.Product.Price.Amount,
			Currency:  it.Product.Price.Currency,
			Quantity:  it.Quantity,
			Available: it.Product.Stock,
			Sellable:  it.Product.Sellable(),
			CreatedAt: it.CreatedAt,
		}
	}

	total, err := c.Total()
	if err != nil {
		return cartResponse{}, err
	}
	out.Total = total.Amount
	return out, nil
}
