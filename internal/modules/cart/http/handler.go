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

// cartResponse is this endpoint's wire contract. UserID is deliberately
// dropped -- the caller is always the authenticated user, so echoing it back
// tells the client nothing it doesn't already know.
//
// Total is a bare amount with no sibling currency key, unlike the per-item
// price/currency pair below. That asymmetry predates money.Money and is
// preserved by flattening Cart.Total() to its Amount here rather than letting
// the type marshal itself -- see internal/money/doc.go.
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

// toCartResponse maps the domain cart onto the wire shape. item.Product is
// guaranteed non-nil here: Service.GetCart sets it to either the looked-up
// product or a synthetic &Product{Status: "unavailable"} placeholder when the
// product record is gone entirely, so this mapper never has to nil-check it.
//
// The total is asked of the cart rather than accumulated in this loop. Summing
// money.Money values can fail -- a cart may hold lines priced in different
// currencies -- and neither the sum nor its failure is a transport concern; see
// cart.Cart.Total. The error is returned rather than swallowed into a zero,
// because a total this adapter could not compute must not be published as one it
// could.
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

// addItemRequest carries the validation rules, moved here verbatim from the
// old cart.AddItemRequest. They live in the transport, not the core: a
// service reachable from a worker should not inherit HTTP's validation
// vocabulary.
type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity"   validate:"required,min=1"`
}

// toAddItemParams is the seam: HTTP's validation vocabulary stops here, and
// the service receives a plain input struct.
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

// updateQuantityRequest carries the validation rules, moved here verbatim
// from the old cart.UpdateItemRequest.
type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

// toUpdateQuantityParams is the seam: HTTP's validation vocabulary stops
// here, and the service receives a plain input struct.
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
