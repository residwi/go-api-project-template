package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// CartReader is what Handler needs from query.Reader: query.Reader satisfies
// it directly, so nothing sits between them, and the mockery-generated mock
// is the other implementation, used in handler_test.go.
type CartReader interface {
	GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
}

type Handler struct {
	reader CartReader
}

func New(reader CartReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("GET /cart", h.get)
}

// UserID is dropped: the caller is always the authenticated user. Total carries
// no currency key, unlike the per-item pair below, and that stays so.
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
	// The line is still returned when unsellable, so the customer can see why
	// their total changed instead of it silently shrinking.
	Sellable  bool      `json:"sellable"`
	CreatedAt time.Time `json:"created_at"`
}

// item.Product needs no nil check: Reader.GetCart substitutes a synthetic
// unavailable placeholder when the record is gone.
//
// The total is asked of the cart, not accumulated here, and its error is
// returned rather than swallowed into a zero -- which would publish a total this
// adapter could not compute.
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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	c, err := h.reader.GetCart(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out, err := toCartResponse(c)
	if err != nil {
		// A mixed-currency cart lands here as a wrapped apperror.ErrBadRequest, so
		// the client sees the 400 checkout would give it rather than a 500.
		response.HandleErr(w, err)
		return
	}

	response.OK(w, out)
}
