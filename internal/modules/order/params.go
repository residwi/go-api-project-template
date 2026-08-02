package order

// PlaceParams is the service's input contract for PlaceOrder. It carries no
// json or validate tags: those belong to a transport. PaymentMethodID is
// here despite the plan's field list omitting it -- PlaceOrder passes it
// straight through to PaymentInitiator.InitiatePayment (service.go), so
// dropping it would silently charge every order with an empty payment
// method.
//
// RetryPayment and AdminUpdateStatus are not given params types: they
// already take plain (paymentMethodID string) / (toStatus Status)
// arguments, not a request struct, so there is no dto-in-the-core cycle to
// break for either.
type PlaceParams struct {
	PaymentMethodID string
	CouponCode      *string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
}
