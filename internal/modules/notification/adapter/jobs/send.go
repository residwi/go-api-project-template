package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

type SendArgs struct {
	NotificationID uuid.UUID
}

func (SendArgs) Kind() string { return "notification.send" }

func (SendArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "notification",
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByQueue: true},
	}
}

type Sender interface {
	Send(ctx context.Context, notificationID uuid.UUID) error
}

type SendWorker struct {
	river.WorkerDefaults[SendArgs]

	service Sender
	timeout time.Duration
}

func NewSendWorker(service Sender, timeout time.Duration) *SendWorker {
	return &SendWorker{service: service, timeout: timeout}
}

func (w *SendWorker) Timeout(*river.Job[SendArgs]) time.Duration { return w.timeout }

func (w *SendWorker) Work(ctx context.Context, job *river.Job[SendArgs]) error {
	return w.service.Send(ctx, job.Args.NotificationID)
}
