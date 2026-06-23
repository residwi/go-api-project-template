package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Notification, error) {
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

// EnqueueOrderPlaced satisfies the order.NotificationEnqueuer interface.
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

// Process creates the notification for a job and owns the job's terminal state
// (the runner does not). On success it persists the notification and completes
// the job atomically, so a lost completion can't re-deliver a duplicate; on
// failure it records the attempt so the job reaches 'failed' after MaxAttempts.
func (s *Service) Process(ctx context.Context, job Job) error {
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
			slog.ErrorContext(ctx, "failed to update notification job after failure", "job_id", job.ID, "error", updateErr)
		}
		return fmt.Errorf("processing notification: %w", err)
	}

	return nil
}
