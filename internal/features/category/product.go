package category

import (
	"context"

	"github.com/google/uuid"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

// ProductCounter guards category deletion. The products.category_id foreign key
// is the backstop -- this gives the caller a useful message before Postgres
// gives them a constraint violation.
type ProductCounter interface {
	CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error)
}
