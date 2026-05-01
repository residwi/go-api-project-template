package email

import (
	"context"
	"log/slog"
)

type Message struct {
	To      string
	Subject string
	Body    string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

type NoopSender struct{}

func (n *NoopSender) Send(ctx context.Context, msg Message) error {
	slog.InfoContext(ctx, "email sent (noop)", "to", msg.To, "subject", msg.Subject)
	return nil
}
