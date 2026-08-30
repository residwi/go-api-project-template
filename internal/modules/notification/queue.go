package notification

import (
	"context"

	"github.com/google/uuid"
)

type Queue interface {
	EnqueueSend(ctx context.Context, notificationID uuid.UUID) error
}
