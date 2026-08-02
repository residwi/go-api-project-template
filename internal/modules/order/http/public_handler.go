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

type publicHandler struct {
	service   *order.Service
	validator *validator.Validator
}

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
//
// Price and Subtotal are int64 minor units with no sibling currency key, even
// though order.Item now holds them as money.Money: this endpoint publishes the
// currency once, on the parent order. Flattening each money.Money to its Amount
// here -- rather than letting the type marshal itself -- is what keeps that
// asymmetry the adapter's decision. See internal/money/doc.go.
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

// orderResponse is shared by every endpoint that returns an order -- public
// and admin alike expose the identical shape. RequestHash, StockDeducted,
// and StockReversed are dropped: RequestHash is an idempotency internal, and
// the stock flags are saga state that would let a client infer fulfilment
// internals if published.
//
// Shared by this file's PlaceOrder (via placeOrderResponse's embedded
// field), ListOrders, and GetOrder, and by admin_handler.go's List and Get:
// placed here because PlaceOrder is the first of those in routes.go's
// registration order.
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
		// The order's three money.Money values are flattened to their amounts, and
		// the currency is emitted once, from Total -- the amount actually charged.
		// All three share a currency by construction, so any of them would do; Total
		// is named because it is the one the client is being billed.
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

// addressRequest maps to order.Address explicitly rather than binding the
// domain struct directly, mirroring addressResponse's rationale.
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
	PaymentMethodID string          `json:"payment_method_id" validate:"required"`
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

// placeOrderResponse keeps the pre-refactor "order" envelope: integration
// tests (internal/transport/http/router_test.go) decode
// data.order.{id,coupon_code,subtotal_amount,discount_amount,total_amount},
// so flattening this to just orderResponse would be a real behavior change,
// not just a shape change.
type placeOrderResponse struct {
	Order orderResponse `json:"order"`
}

func toPlaceOrderResponse(r *order.PlaceResult) placeOrderResponse {
	return placeOrderResponse{Order: toOrderResponse(r.Order)}
}

func (h *publicHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
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

func (h *publicHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
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

// payRequest has no params.go counterpart: order.Service.RetryPayment
// already takes a plain string, not a request struct, so there is no
// dto-in-the-core cycle to break here.
type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

// payResultResponse gives order.PaymentResult proper snake_case wire tags --
// the pre-refactor handler serialized it untagged (ports.go carries no json
// tags at all), so this also fixes a latent inconsistency with every other
// response in the app, not just a rename.
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

func (h *publicHandler) RetryPayment(w http.ResponseWriter, r *http.Request) {
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

func (h *publicHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
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
