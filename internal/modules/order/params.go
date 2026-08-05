package order

type PlaceParams struct {
	PaymentMethodID string
	CouponCode      *string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
}
