package jobs

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type ChargeProcessor interface {
	ProcessCharge(ctx context.Context, job domain.Job) error
}

type RefundProcessor interface {
	ProcessRefund(ctx context.Context, job domain.Job) error
}
