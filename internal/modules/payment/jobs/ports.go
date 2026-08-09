package jobs

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// ChargeProcessor and RefundProcessor are what Dispatcher.Process routes a
// claimed job to. charge and refund each own the mechanics of their own
// action -- jobs only owns the queue -- so these are satisfied by
// charge.Command and refund.Command directly, wired into a Dispatcher (see
// dispatcher.go) in payment/module.go after both exist.
type ChargeProcessor interface {
	ProcessCharge(ctx context.Context, job domain.Job) error
}

type RefundProcessor interface {
	ProcessRefund(ctx context.Context, job domain.Job) error
}
