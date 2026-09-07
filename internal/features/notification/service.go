package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo    Repository
	tx      database.TxRunner
	queue   JobQueue
	channel Channel
	logger  *slog.Logger
	tracer  trace.Tracer
}

func New(
	repo Repository,
	tx database.TxRunner,
	queue JobQueue,
	channel Channel,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:    repo,
		tx:      tx,
		queue:   queue,
		channel: channel,
		logger:  logger,
		tracer:  otel.Tracer("github.com/residwi/go-api-project-template/internal/features/notification"),
	}
}

func (s *Service) Create(ctx context.Context, in NewNotification) (err error) {
	ctx, span := s.tracer.Start(ctx, "notification.Create")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.tx.Run(ctx, func(txCtx context.Context) error {
		n := &domain.Notification{UserID: in.UserID, Title: in.Title, Body: in.Body}
		if err := s.repo.Create(txCtx, n); err != nil {
			return err
		}

		return s.queue.EnqueueSend(txCtx, n.ID)
	})
}

func (s *Service) Send(ctx context.Context, notificationID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "notification.Send")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	n, err := s.repo.Get(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("getting notification %s: %w", notificationID, err)
	}
	return s.channel.Send(ctx, n)
}

func (s *Service) List(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) (_ []domain.Notification, err error) {
	ctx, span := s.tracer.Start(ctx, "notification.List")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (_ int, err error) {
	ctx, span := s.tracer.Start(ctx, "notification.CountUnread")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.CountUnread(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "notification.MarkRead")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "notification.MarkAllRead")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.MarkAllRead(ctx, userID)
}
