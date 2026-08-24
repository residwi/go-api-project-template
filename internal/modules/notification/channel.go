package notification

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
)

type Channel interface {
	Send(ctx context.Context, n domain.Notification) error
}
