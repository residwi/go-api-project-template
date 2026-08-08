package remove

import (
	"context"

	"github.com/google/uuid"
)

// ProductCounter guards category deletion. The products.category_id foreign key
// is the backstop -- this gives the caller a useful message before Postgres
// gives them a constraint violation.
type ProductCounter interface {
	CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error)
}
