package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *order.Service
	validator *validator.Validator
}

// Mapped explicitly rather than reusing order.Address, so a field added there
// later cannot ride onto the wire unnoticed.
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

// Drops OrderID: an internal join key, and the item is always nested inside the
// order the client asked for.
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

type addressRequest struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func (r *addressRequest) toAddress() *order.Address {
	if r == nil {
		return nil
	}
	return &order.Address{
		Street:  r.Street,
		City:    r.City,
		State:   r.State,
		ZipCode: r.ZipCode,
		Country: r.Country,
	}
}

type placeOrderRequest struct {
	PaymentMethodID string          `json:"payment_method_id"          validate:"required"`
	CouponCode      *string         `json:"coupon_code,omitempty"`
	ShippingAddress *addressRequest `json:"shipping_address,omitempty"`
	BillingAddress  *addressRequest `json:"billing_address,omitempty"`
	Notes           string          `json:"notes,omitempty"`
}

func (r placeOrderRequest) toPlaceParams() order.PlaceParams {
	return order.PlaceParams{
		PaymentMethodID: r.PaymentMethodID,
		CouponCode:      r.CouponCode,
		ShippingAddress: r.ShippingAddress.toAddress(),
		BillingAddress:  r.BillingAddress.toAddress(),
		Notes:           r.Notes,
	}
}

// The "order" envelope is load-bearing: clients decode data.order.{...}, so
// flattening this to a bare orderResponse breaks them.
type placeOrderResponse struct {
	Order orderResponse `json:"order"`
}

func toPlaceOrderResponse(r *order.PlaceResult) placeOrderResponse {
	return placeOrderResponse{Order: toOrderResponse(r.Order)}
}

func (h *handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		response.BadRequest(w, "Idempotency-Key header is required")
		return
	}

	req, ok := response.Bind[placeOrderRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.PlaceOrder(r.Context(), uc.UserID, req.toPlaceParams(), idempotencyKey)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toPlaceOrderResponse(result))
}

func (h *handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	orders, err := h.service.ListByUser(r.Context(), uc.UserID, cursor)
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

func (h *handler) GetOrder(w http.ResponseWriter, r *http.Request) {
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

type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r *order.PaymentResult) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}

func (h *handler) RetryPayment(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[payRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.RetryPayment(r.Context(), uc.UserID, id, req.PaymentMethodID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toPayResultResponse(result))
}

func (h *handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.CancelOrder(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
