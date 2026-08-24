package log

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
)

var _ notification.Channel = (*Channel)(nil)

type Channel struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Channel {
	return &Channel{logger: logger}
}

func (c *Channel) Send(ctx context.Context, n domain.Notification) error {
	c.logger.InfoContext(ctx, "notification sent",
		slog.String("notification_id", n.ID.String()),
		slog.String("user_id", n.UserID.String()),
		slog.String("title", n.Title))
	return nil
}
