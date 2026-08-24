package notification

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type SendJob struct {
	NotificationID uuid.UUID

	svc *Service
}

func NewSendJob(s *Service) SendJob { return SendJob{svc: s} }

func (SendJob) Kind() string { return "notification.send" }

func (j SendJob) Run(ctx context.Context) error {
	n, err := j.svc.repo.Get(ctx, j.NotificationID)
	if err != nil {
		return fmt.Errorf("getting notification %s: %w", j.NotificationID, err)
	}
	return j.svc.channel.Send(ctx, n)
}
