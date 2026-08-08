package markallread

import (
	"context"

	"github.com/google/uuid"
)

// Repository is markallread's own storage. Its only implementation is
// markallread/postgres, constructed in notification/module.go.
type Repository interface {
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}
