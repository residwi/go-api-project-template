// Package domain holds category's aggregate and its rules. It is
// module-private: what leaves category leaves through a slice's return type.
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
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

// ValidateParentSelf rejects a category naming itself as its own parent.
// Split from ValidateParentDepth because create and update both check this
// before calling their own AncestorDepthAndCycle, not after: it needs no
// database round trip, so neither slice should pay for one just to learn
// what parentID == selfID already told it.
func ValidateParentSelf(parentID, selfID uuid.UUID) error {
	if parentID == selfID && selfID != uuid.Nil {
		return fmt.Errorf("%w: category cannot be its own parent", apperror.ErrBadRequest)
	}
	return nil
}

// ValidateParentDepth is the rule create and update both enforce once they
// have a parent's ancestry: a missing parent, a cycle, and a chain past
// MaxDepth are all rejected. depth and formsCycle are the facts each slice's
// own AncestorDepthAndCycle already computed through its own Repository
// port -- the I/O stays in the slice, only the interpretation of its answer
// lives here, so the two callers cannot drift into reporting two different
// messages for the same violation.
func ValidateParentDepth(depth int, formsCycle bool) error {
	if depth == 0 {
		return fmt.Errorf("%w: parent category not found", apperror.ErrBadRequest)
	}
	if formsCycle {
		return fmt.Errorf("%w: circular parent reference", apperror.ErrBadRequest)
	}
	// depth is the distance from parent to root. Adding this child makes it depth+1.
	if depth+1 > MaxDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", apperror.ErrBadRequest, MaxDepth)
	}

	return nil
}
