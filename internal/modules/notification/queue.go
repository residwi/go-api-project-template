package notification

import (
	"context"

	"github.com/google/uuid"
)

type SendQueue interface {
	EnqueueSend(ctx context.Context, notificationID uuid.UUID) error
}
