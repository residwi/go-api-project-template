package payment

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is payment's persistence port. The Postgres implementation
// lives in the postgres subpackage; this package never imports it.
type Repository interface {
	Create(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	GetActiveByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error)
	GetByGatewayTxnID(ctx context.Context, txnID string) (*Payment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, toStatus Status, fromStatuses []Status) error
	UpdateGateway(ctx context.Context, id uuid.UUID, txnID string, response []byte) error
	UpdatePaymentURL(ctx context.Context, id uuid.UUID, paymentURL string) error
	ClearPaymentURL(ctx context.Context, id uuid.UUID) error
	MarkPaid(ctx context.Context, id uuid.UUID, fromStatuses []Status) error
	ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]Payment, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]Payment, int, error)
	CreateJob(ctx context.Context, job *Job) error
	Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]Job, error)
	UpdateJob(ctx context.Context, job *Job) error
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
	MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action JobAction) error
	Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error)
}

type AdminListParams struct {
	Page     int
	PageSize int
	Status   string
	OrderID  string
}
