package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type Worker struct {
	repo   Repository
	logger *slog.Logger
}

func New(repo Repository, log *slog.Logger) *Worker {
	return &Worker{repo: repo, logger: log}
}

func (w *Worker) Claim(ctx context.Context, batch int, lease time.Duration) ([]domain.Job, error) {
	return w.repo.Claim(ctx, batch, lease)
}

func (w *Worker) Prune(ctx context.Context, age time.Duration, limit int) (int, error) {
	return w.repo.Prune(ctx, age, limit)
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
	return w.repo.CreateJob(ctx, job)
}

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
	if err := w.repo.CreateAndComplete(ctx, n, &job); err != nil {
		job.Attempts++
		job.LastError = err.Error()
		if job.Attempts >= job.MaxAttempts {
			job.Status = domain.JobStatusFailed
		} else {
			job.Status = domain.JobStatusPending
		}
		if updateErr := w.repo.UpdateJob(ctx, &job); updateErr != nil {
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
