package order

import (
	"context"
	"log/slog"
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/jobs"
)

type ExpireStaleJob struct {
	At    time.Time
	Every time.Duration

	svc *Service
}

func NewExpireStaleJob(s *Service) ExpireStaleJob { return ExpireStaleJob{svc: s} }

func (ExpireStaleJob) Kind() string { return "order.expire-stale" }

func (j ExpireStaleJob) Run(ctx context.Context) error {
	if err := j.svc.scheduleExpireStale(ctx, j.At.Add(j.Every), j.Every); err != nil {
		return err
	}

	if err := j.svc.RecoverStale(ctx); err != nil {
		j.svc.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return j.svc.ExpireStale(ctx)
}

func (s *Service) scheduleExpireStale(ctx context.Context, at time.Time, every time.Duration) error {
	slot := at.UTC().Truncate(every)

	return jobs.Enqueue(ctx, s.queue, ExpireStaleJob{At: slot, Every: every}, jobs.Keys{
		Dedup: "order.expire-stale:" + slot.Format(time.RFC3339),
	}, jobs.RunAt(slot))
}

func (s *Service) ScheduleExpireStale(ctx context.Context, every time.Duration) error {
	return s.scheduleExpireStale(ctx, time.Now().Add(every), every)
}
