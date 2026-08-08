package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

// Worker owns every operation on the queue table: order/place enqueues
// through EnqueueOrderPlaced, and platform/jobs.Runner drains it through the
// embedded Repository's Claim and Prune plus Process below. One value
// satisfying both platform/jobs' Queue and Processor is why notification
// still needs no separate worker/ package.
type Worker struct {
	Repository

	logger *slog.Logger
}

func New(repo Repository, log *slog.Logger) *Worker {
	return &Worker{Repository: repo, logger: log}
}

func (w *Worker) EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error {
	job := &domain.Job{
		UserID:      userID,
		Type:        string(domain.TypeOrderPlaced),
		Title:       "Order Placed",
		Body:        fmt.Sprintf("Your order %s has been placed.", orderID.String()),
		Status:      domain.JobStatusPending,
		Attempts:    0,
		MaxAttempts: 3,
	}
	return w.CreateJob(ctx, job)
}

// Process owns the job's terminal state, not the runner. Notification and
// completion commit atomically, so a lost completion cannot re-deliver.
func (w *Worker) Process(ctx context.Context, job domain.Job) error {
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	n := &domain.Notification{
		UserID: job.UserID,
		Type:   domain.Type(job.Type),
		Title:  job.Title,
		Body:   job.Body,
		IsRead: false,
		Data:   job.Data,
	}

	job.Status = domain.JobStatusCompleted
	if err := w.CreateAndComplete(ctx, n, &job); err != nil {
		// Record the attempt so the job retries and reaches 'failed' after MaxAttempts.
		job.Attempts++
		job.LastError = err.Error()
		if job.Attempts >= job.MaxAttempts {
			job.Status = domain.JobStatusFailed
		} else {
			job.Status = domain.JobStatusPending
		}
		if updateErr := w.UpdateJob(ctx, &job); updateErr != nil {
			w.logger.ErrorContext(
				ctx,
				"failed to update notification job after failure",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("processing notification: %w", err)
	}

	return nil
}
