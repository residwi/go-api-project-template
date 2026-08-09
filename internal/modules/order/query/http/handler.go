package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// Reader is what Handler needs from query.Reader: query.Reader satisfies it
// directly, so nothing sits between them, and the mockery-generated mock is
// the other implementation, used in handler_test.go.
type Reader interface {
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error)
	GetByIDForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Order, error)
}

type Handler struct {
	reader Reader
}

func New(reader Reader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("GET /orders", h.list)
	authed.HandleFunc("GET /orders/{id}", h.get)
}

// Mapped explicitly rather than reusing domain.Address, so a field added there
// later cannot ride onto the wire unnoticed.
type addressResponse struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func toAddressResponse(a *domain.Address) *addressResponse {
	if a == nil {
		return nil
	}
	return &addressResponse{
		Street:  a.Street,
		City:    a.City,
		State:   a.State,
		ZipCode: a.ZipCode,
		Country: a.Country,
	}
}

// Drops OrderID: an internal join key, and the item is always nested inside
// the order the client asked for.
type orderItemResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Quantity    int       `json:"quantity"`
	Subtotal    int64     `json:"subtotal"`
	CreatedAt   time.Time `json:"created_at"`
}

func toOrderItemResponse(i domain.Item) orderItemResponse {
	return orderItemResponse{
		ID:          i.ID,
		ProductID:   i.ProductID,
		ProductName: i.ProductName,
		Price:       i.Price.Amount,
		Quantity:    i.Quantity,
		Subtotal:    i.Subtotal.Amount,
		CreatedAt:   i.CreatedAt,
	}
}

// Shared by the public and admin endpoints, which expose the identical shape.
// RequestHash is an idempotency internal; StockDeducted and StockReversed are
// saga state a client could read fulfilment internals from.
type orderResponse struct {
	ID              uuid.UUID           `json:"id"`
	UserID          uuid.UUID           `json:"user_id"`
	Status          domain.Status       `json:"status"`
	SubtotalAmount  int64               `json:"subtotal_amount"`
	DiscountAmount  int64               `json:"discount_amount"`
	TotalAmount     int64               `json:"total_amount"`
	CouponCode      *string             `json:"coupon_code,omitempty"`
	Currency        string              `json:"currency"`
	ShippingAddress *addressResponse    `json:"shipping_address,omitempty"`
	BillingAddress  *addressResponse    `json:"billing_address,omitempty"`
	Notes           string              `json:"notes,omitempty"`
	Items           []orderItemResponse `json:"items,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

func toOrderResponse(o *domain.Order) orderResponse {
	items := make([]orderItemResponse, len(o.Items))
	for i, it := range o.Items {
		items[i] = toOrderItemResponse(it)
	}

	return orderResponse{
		ID:     o.ID,
		UserID: o.UserID,
		Status: o.Status,
		// Total's currency is the one published: it is the amount the client is billed.
		SubtotalAmount:  o.Subtotal.Amount,
		DiscountAmount:  o.Discount.Amount,
		TotalAmount:     o.Total.Amount,
		CouponCode:      o.CouponCode,
		Currency:        o.Total.Currency,
		ShippingAddress: toAddressResponse(o.ShippingAddress),
		BillingAddress:  toAddressResponse(o.BillingAddress),
		Notes:           o.Notes,
		Items:           items,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	orders, err := h.reader.ListByUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]orderResponse, len(orders))
	for i, o := range orders {
		out[i] = toOrderResponse(&o)
	}

	response.CursorPage(w, out, cursor.Limit, func(o orderResponse) (time.Time, uuid.UUID) {
		return o.CreatedAt, o.ID
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.reader.GetByIDForUser(r.Context(), uc.UserID, id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}
