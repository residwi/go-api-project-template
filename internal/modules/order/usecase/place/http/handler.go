package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/place"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type OrderPlacer interface {
	Execute(ctx context.Context, userID uuid.UUID, p place.Params, idempotencyKey string) (*place.Result, error)
}

type Handler struct {
	cmd       OrderPlacer
	validator *validator.Validator
}

func New(cmd OrderPlacer, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

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
		ID:              o.ID,
		UserID:          o.UserID,
		Status:          o.Status,
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

func (r *addressRequest) toAddress() *domain.Address {
	if r == nil {
		return nil
	}
	return &domain.Address{
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

func (r placeOrderRequest) toParams() place.Params {
	return place.Params{
		PaymentMethodID: r.PaymentMethodID,
		CouponCode:      r.CouponCode,
		ShippingAddress: r.ShippingAddress.toAddress(),
		BillingAddress:  r.BillingAddress.toAddress(),
		Notes:           r.Notes,
	}
}

type placeOrderResponse struct {
	Order orderResponse `json:"order"`
}

func toPlaceOrderResponse(r *place.Result) placeOrderResponse {
	return placeOrderResponse{Order: toOrderResponse(r.Order)}
}

func (h *Handler) Place(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.cmd.Execute(r.Context(), uc.UserID, req.toParams(), idempotencyKey)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toPlaceOrderResponse(result))
}
