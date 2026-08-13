package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error) {
	return c.repo.Claim(ctx, batchSize, leaseDuration)
}

func (c *Command) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	return c.repo.Prune(ctx, olderThan, limit)
}

func (c *Command) CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return c.repo.CancelJobsByOrderID(ctx, orderID)
}

func (c *Command) MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error {
	return c.repo.MarkJobCompleted(ctx, jobID)
}

func (c *Command) MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error {
	return c.repo.MarkJobCompletedByPaymentID(ctx, paymentID, action)
}

func (c *Command) UpdateJob(ctx context.Context, job *domain.Job) error {
	return c.repo.UpdateJob(ctx, job)
}

func (c *Command) EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	return c.repo.CreateJob(ctx, &domain.Job{
		PaymentID:   paymentID,
		OrderID:     orderID,
		Action:      domain.ActionRefund,
		Status:      domain.JobStatusPending,
		NextRetryAt: time.Now(),
	})
}
