package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Service struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, logger: log}
}

func (s *Service) Send(ctx context.Context, userID uuid.UUID, typ Type, title, body string, data []byte) error {
	n := &Notification{
		UserID: userID,
		Type:   typ,
		Title:  title,
		Body:   body,
		IsRead: false,
		Data:   data,
	}
	return s.repo.Create(ctx, n)
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]Notification, error) {
	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *Service) EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error {
	job := &Job{
		UserID:      userID,
		Type:        string(TypeOrderPlaced),
		Title:       "Order Placed",
		Body:        fmt.Sprintf("Your order %s has been placed.", orderID.String()),
		Status:      JobStatusPending,
		Attempts:    0,
		MaxAttempts: 3,
	}
	return s.repo.CreateJob(ctx, job)
}

// Process owns the job's terminal state, not the runner. Notification and
// completion commit atomically, so a lost completion cannot re-deliver.
func (s *Service) Process(ctx context.Context, job Job) error {
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	n := &Notification{
		UserID: job.UserID,
		Type:   Type(job.Type),
		Title:  job.Title,
		Body:   job.Body,
		IsRead: false,
		Data:   job.Data,
	}

	job.Status = JobStatusCompleted
	if err := s.repo.CreateAndComplete(ctx, n, &job); err != nil {
		// Record the attempt so the job retries and reaches 'failed' after MaxAttempts.
		job.Attempts++
		job.LastError = err.Error()
		if job.Attempts >= job.MaxAttempts {
			job.Status = JobStatusFailed
		} else {
			job.Status = JobStatusPending
		}
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update notification job after failure",
				slog.Any("error", updateErr),
			)
		}
		return fmt.Errorf("processing notification: %w", err)
	}

	return nil
}
