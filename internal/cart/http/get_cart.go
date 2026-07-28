package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/cart"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// cartResponse is this endpoint's wire contract. UserID is deliberately
// dropped -- the caller is always the authenticated user, so echoing it back
// tells the client nothing it doesn't already know.
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
	// Sellable is false when the product was archived, unpublished, or removed
	// after this line was added. The line is still returned so the customer can
	// see why their total changed, instead of it silently shrinking.
	Sellable  bool      `json:"sellable"`
	CreatedAt time.Time `json:"created_at"`
}

// toCartResponse maps the domain cart onto the wire shape and computes the
// total from sellable lines only. item.Product is guaranteed non-nil here:
// Service.GetCart sets it to either the looked-up product or a synthetic
// &Product{Status: "unavailable"} placeholder when the product record is
// gone entirely, so this mapper never has to nil-check it.
func toCartResponse(c *cart.Cart) cartResponse {
	out := cartResponse{ID: c.ID, Items: make([]cartItemResponse, len(c.Items))}
	for i, it := range c.Items {
		sellable := it.Product.Sellable()
		out.Items[i] = cartItemResponse{
			ID:        it.ID,
			ProductID: it.ProductID,
			Name:      it.Product.Name,
			Price:     it.Product.Price,
			Currency:  it.Product.Currency,
			Quantity:  it.Quantity,
			Available: it.Product.Stock,
			Sellable:  sellable,
			CreatedAt: it.CreatedAt,
		}
		if sellable {
			out.Total += it.Product.Price * int64(it.Quantity)
		}
	}
	return out
}

func (h *handler) GetCart(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	c, err := h.service.GetCart(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toCartResponse(c))
}
