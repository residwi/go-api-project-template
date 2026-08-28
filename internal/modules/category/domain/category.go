package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

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
		return fmt.Errorf("%w: category cannot be its own parent", errs.ErrBadRequest)
	}
	return nil
}

func ValidateParentDepth(depth int, formsCycle bool) error {
	if depth == 0 {
		return fmt.Errorf("%w: parent category not found", errs.ErrBadRequest)
	}
	if formsCycle {
		return fmt.Errorf("%w: circular parent reference", errs.ErrBadRequest)
	}
	if depth+1 > MaxDepth {
		return fmt.Errorf("%w: category depth exceeds maximum of %d", errs.ErrBadRequest, MaxDepth)
	}

	return nil
}
