package http

import (
	"github.com/google/uuid"
)

type createCategoryRequest struct {
	Name        string     `json:"name"        validate:"required,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id"   validate:"omitempty"`
	SortOrder   *int       `json:"sort_order"  validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

type updateCategoryRequest struct {
	Name        *string    `json:"name"        validate:"omitempty,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id"   validate:"omitempty"`
	SortOrder   *int       `json:"sort_order"  validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}
