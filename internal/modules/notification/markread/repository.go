package markread

import (
	"context"

	"github.com/google/uuid"
)

// Repository is markread's own storage. Its only implementation is
// markread/postgres, constructed in notification/module.go.
type Repository interface {
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
}
