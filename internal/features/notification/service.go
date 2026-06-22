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

// Process creates a notification record from a job and owns the job's terminal
// state. The runner does not mark jobs done, so Process must: on success mark the
// job completed (otherwise its lease lapses and it is re-claimed, creating
// duplicate notifications forever), and on failure record the attempt so the job
// reaches a terminal 'failed' state after MaxAttempts instead of being retried
// indefinitely with attempts frozen at zero.
func (s *Service) Process(ctx context.Context, job Job) error {
	n := &Notification{
		UserID: job.UserID,
		Type:   Type(job.Type),
		Title:  job.Title,
		Body:   job.Body,
		IsRead: false,
		Data:   job.Data,
	}

	if err := s.repo.Create(ctx, n); err != nil {
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
		return fmt.Errorf("creating notification: %w", err)
	}

	job.Status = JobStatusCompleted
	if err := s.repo.UpdateJob(ctx, &job); err != nil {
		slog.ErrorContext(ctx, "failed to mark notification job completed", "job_id", job.ID, "error", err)
	}
	return nil
}
