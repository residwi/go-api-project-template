package notification

import (
	"context"

	"github.com/google/uuid"
)

type JobQueue interface {
	EnqueueSend(ctx context.Context, notificationID uuid.UUID) error
}
