// Package domain holds category's aggregate and its rules. It is
// module-private: what leaves category leaves through a slice's return type.
package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

// MaxDepth bounds how many ancestors a category chain may have. create and
// update both walk the chain before writing a parent_id, so the number lives
// here once rather than as a literal `5` repeated in each slice.
const MaxDepth = 5

type Category struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   int
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Slugify derives a category's slug from its name, falling back to a
// "category-"-prefixed seed when the name has no ASCII alphanumerics to
// slugify (e.g. an all-CJK or all-emoji name). create and update each pick
// their own seed -- a fresh id before the row exists, the row's own id after.
func Slugify(name, fallbackSeed string) string {
	return slug.MakeOrFallback(name, "category-"+fallbackSeed)
}
