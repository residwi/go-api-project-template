package email

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopSender_Send(t *testing.T) {
	sender := &NoopSender{}
	err := sender.Send(context.Background(), Message{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	assert.NoError(t, err)
}
