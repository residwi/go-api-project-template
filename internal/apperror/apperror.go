package apperror

import (
	"fmt"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var (
	ErrInsufficientStock = fmt.Errorf("%w: insufficient stock", errs.ErrConflict)
	ErrCartEmpty         = fmt.Errorf("%w: cart is empty", errs.ErrBadRequest)
	ErrOrderNotPayable   = fmt.Errorf("%w: order is not in payable state", errs.ErrBadRequest)
	ErrOrderCharging     = fmt.Errorf("%w: order has an in-flight payment, cannot cancel", errs.ErrConflict)
	ErrAmountMismatch    = fmt.Errorf("%w: payment amount does not match order total", errs.ErrConflict)
	ErrCouponExhausted   = fmt.Errorf("%w: coupon usage limit reached", errs.ErrConflict)
	ErrAlreadyFinalized  = fmt.Errorf("%w: payment already finalized", errs.ErrConflict)
)
