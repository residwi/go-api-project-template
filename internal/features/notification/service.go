package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Service struct {
	repo    Repository
	tx      database.TxRunner
	queue   Queue
	channel Channel
	logger  *slog.Logger
}

func New(
	repo Repository,
	tx database.TxRunner,
	queue Queue,
	channel Channel,
	logger *slog.Logger,
) *Service {
	return &Service{repo: repo, tx: tx, queue: queue, channel: channel, logger: logger}
}

func (s *Service) Create(ctx context.Context, in NewNotification) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		n := &domain.Notification{UserID: in.UserID, Title: in.Title, Body: in.Body}
		if err := s.repo.Create(txCtx, n); err != nil {
			return err
		}

		return s.queue.EnqueueSend(txCtx, n.ID)
	})
}

func (s *Service) Send(ctx context.Context, notificationID uuid.UUID) error {
	n, err := s.repo.Get(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("getting notification %s: %w", notificationID, err)
	}
	return s.channel.Send(ctx, n)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Notification, error) {
	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}
