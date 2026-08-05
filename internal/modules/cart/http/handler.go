package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *cart.Service
	validator *validator.Validator
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

// item.Product needs no nil check: Service.GetCart substitutes a synthetic
// unavailable placeholder when the record is gone.
//
// The total is asked of the cart, not accumulated here, and its error is
// returned rather than swallowed into a zero -- which would publish a total this
// adapter could not compute.
func toCartResponse(c *cart.Cart) (cartResponse, error) {
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

	out, err := toCartResponse(c)
	if err != nil {
		// A mixed-currency cart lands here as a wrapped apperror.ErrBadRequest, so
		// the client sees the 400 checkout would give it rather than a 500.
		response.HandleErr(w, err)
		return
	}

	response.OK(w, out)
}

// Validation lives in the transport: a service reachable from a worker should
// not inherit HTTP's validation vocabulary.
type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity"   validate:"required,min=1"`
}

func (r addItemRequest) toAddItemParams() cart.AddItemParams {
	return cart.AddItemParams{ProductID: r.ProductID, Quantity: r.Quantity}
}

func (h *handler) AddItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.AddItem(r.Context(), uc.UserID, req.toAddItemParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

func (r updateQuantityRequest) toUpdateQuantityParams() cart.UpdateQuantityParams {
	return cart.UpdateQuantityParams{Quantity: r.Quantity}
}

func (h *handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateQuantityRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req.toUpdateQuantityParams()); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.service.RemoveItem(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) Clear(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
