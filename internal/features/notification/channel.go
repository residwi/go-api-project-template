package notification

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
)

type Channel interface {
	Send(ctx context.Context, n domain.Notification) error
}
