package promotion

import (
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type CreateParams struct {
	Code           string
	Type           Type
	Value          int64
	MinOrderAmount int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       time.Time
	ExpiresAt      time.Time
	Active         bool
}

type UpdateParams struct {
	Code           string
	Type           Type
	Value          *int64
	MinOrderAmount *int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       *time.Time
	ExpiresAt      *time.Time
	Active         *bool
}

type ListParams struct {
	paging.OffsetPage
}
