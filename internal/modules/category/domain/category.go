package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

// MaxDepth bounds how many ancestors a category chain may have. create and
// update both walk the chain before writing a parent_id, so the number lives.
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

func Slugify(name, fallbackSeed string) string {
	return slug.MakeOrFallback(name, "category-"+fallbackSeed)
}

func ValidateParentSelf(parentID, selfID uuid.UUID) error {
	if parentID == selfID && selfID != uuid.Nil {
		return fmt.Errorf("%w: category cannot be its own parent", apperror.ErrBadRequest)
	}
	return nil
}

func ValidateParentDepth(depth int, formsCycle bool) error {
	if depth == 0 {
		return fmt.Errorf("%w: parent category not found", apperror.ErrBadRequest)
	}
	if formsCycle {
		return fmt.Errorf("%w: circular parent reference", apperror.ErrBadRequest)
	}
	if depth+1 > MaxDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", apperror.ErrBadRequest, MaxDepth)
	}

	return nil
}
