package notification

import (
	"context"
	"fmt"

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

func (s *Service) MarkRead(ctx context.Context, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, id)
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

// ProcessJob creates a notification record from a job.
func (s *Service) ProcessJob(ctx context.Context, job Job) error {
	n := &Notification{
		UserID: job.UserID,
		Type:   Type(job.Type),
		Title:  job.Title,
		Body:   job.Body,
		IsRead: false,
		Data:   job.Data,
	}
	return s.repo.Create(ctx, n)
}
