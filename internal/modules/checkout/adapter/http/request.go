package http

import (
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type addressRequest struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func (r *addressRequest) toAddress() *orderdomain.Address {
	if r == nil {
		return nil
	}
	return &orderdomain.Address{
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

func (r placeOrderRequest) toOrder() orderdomain.NewOrder {
	return orderdomain.NewOrder{
		CouponCode:      r.CouponCode,
		ShippingAddress: r.ShippingAddress.toAddress(),
		BillingAddress:  r.BillingAddress.toAddress(),
		Notes:           r.Notes,
	}
}

type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}
