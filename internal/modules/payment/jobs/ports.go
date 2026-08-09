package jobs

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// ChargeProcessor and RefundProcessor are what Process dispatches a claimed
// job to. charge and refund each own the mechanics of their own action --
// jobs only owns the queue -- so these are satisfied by charge.Command and
// refund.Command directly, wired in payment/module.go by SetProcessors after
// both exist: at slice granularity jobs, charge and refund form a genuine
// three-way cycle (charge and refund each need jobs back, to enqueue a
// follow-up job or complete their own), which one setter inside payment's own
// composition breaks. This is not the order/payment setter the rest of this
// task deletes -- that one crossed a module boundary bootstrap had to wire
// after construction; this one is payment's own internal composition, never
// visible outside payment/module.go.
type ChargeProcessor interface {
	ProcessCharge(ctx context.Context, job domain.Job) error
}

type RefundProcessor interface {
	ProcessRefund(ctx context.Context, job domain.Job) error
}
