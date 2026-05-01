package order

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

type PlaceOrderRequest struct {
	PaymentMethodID string        `json:"payment_method_id" validate:"required"`
	CouponCode      *string       `json:"coupon_code,omitempty"`
	ShippingAddress *core.Address `json:"shipping_address,omitempty"`
	BillingAddress  *core.Address `json:"billing_address,omitempty"`
	Notes           string        `json:"notes,omitempty"`
}

type PlaceResponse struct {
	Order *Order `json:"order"`
}

type PayRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

type Response struct {
	ID              uuid.UUID      `json:"id"`
	UserID          uuid.UUID      `json:"user_id"`
	Status          Status         `json:"status"`
	SubtotalAmount  int64          `json:"subtotal_amount"`
	DiscountAmount  int64          `json:"discount_amount"`
	TotalAmount     int64          `json:"total_amount"`
	CouponCode      *string        `json:"coupon_code,omitempty"`
	Currency        string         `json:"currency"`
	ShippingAddress *core.Address  `json:"shipping_address,omitempty"`
	BillingAddress  *core.Address  `json:"billing_address,omitempty"`
	Notes           string         `json:"notes,omitempty"`
	Items           []ItemResponse `json:"items,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type ItemResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Quantity    int       `json:"quantity"`
	Subtotal    int64     `json:"subtotal"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdminUpdateStatusRequest struct {
	Status string `json:"status" validate:"required"`
}
