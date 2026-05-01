package core

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("already exists")
	ErrBadRequest          = errors.New("bad request")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrTokenExpired        = errors.New("token expired")
	ErrInvalidToken        = errors.New("invalid token")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrCartEmpty           = errors.New("cart is empty")
	ErrOrderNotPayable     = errors.New("order is not in payable state")
	ErrOrderCharging       = errors.New("order has an in-flight payment, cannot cancel")
	ErrAmountMismatch      = errors.New("payment amount does not match order total")
	ErrCouponExhausted     = errors.New("coupon usage limit reached")
	ErrFulfillmentFailed   = errors.New("fulfillment failed, refund required")
	ErrAlreadyFinalized    = errors.New("payment already finalized")
	ErrReaderNotConfigured = errors.New("reader database not configured")
)
