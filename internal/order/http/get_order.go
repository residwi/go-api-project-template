package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/order"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// addressResponse maps order.Address explicitly rather than reusing the
// domain struct, per the plan's convention for this feature -- Address
// carries no internal fields today, but a future one (e.g. a
// geocoding-provider id) would otherwise ride onto the wire unnoticed.
type addressResponse struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func toAddressResponse(a *order.Address) *addressResponse {
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

// orderItemResponse drops OrderID: it is an internal join key (the item is
// always returned nested inside its order), and a client already knows
// which order it asked for.
type orderItemResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Quantity    int       `json:"quantity"`
	Subtotal    int64     `json:"subtotal"`
	CreatedAt   time.Time `json:"created_at"`
}

func toOrderItemResponse(i order.Item) orderItemResponse {
	return orderItemResponse{
		ID:          i.ID,
		ProductID:   i.ProductID,
		ProductName: i.ProductName,
		Price:       i.Price,
		Quantity:    i.Quantity,
		Subtotal:    i.Subtotal,
		CreatedAt:   i.CreatedAt,
	}
}

// orderResponse is shared by every endpoint that returns an order -- public
// and admin alike expose the identical shape. RequestHash, StockDeducted,
// and StockReversed are dropped: RequestHash is an idempotency internal, and
// the stock flags are saga state that would let a client infer fulfilment
// internals if published.
type orderResponse struct {
	ID              uuid.UUID           `json:"id"`
	UserID          uuid.UUID           `json:"user_id"`
	Status          order.Status        `json:"status"`
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

func toOrderResponse(o *order.Order) orderResponse {
	items := make([]orderItemResponse, len(o.Items))
	for i, it := range o.Items {
		items[i] = toOrderItemResponse(it)
	}

	return orderResponse{
		ID:              o.ID,
		UserID:          o.UserID,
		Status:          o.Status,
		SubtotalAmount:  o.SubtotalAmount,
		DiscountAmount:  o.DiscountAmount,
		TotalAmount:     o.TotalAmount,
		CouponCode:      o.CouponCode,
		Currency:        o.Currency,
		ShippingAddress: toAddressResponse(o.ShippingAddress),
		BillingAddress:  toAddressResponse(o.BillingAddress),
		Notes:           o.Notes,
		Items:           items,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	}
}

func (h *publicHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.service.GetByID(r.Context(), uc.UserID, id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}
