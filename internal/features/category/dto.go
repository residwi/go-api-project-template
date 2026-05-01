package category

import "github.com/google/uuid"

type CreateCategoryRequest struct {
	Name        string     `json:"name" validate:"required,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id" validate:"omitempty"`
	SortOrder   *int       `json:"sort_order" validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

type UpdateCategoryRequest struct {
	Name        *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id" validate:"omitempty"`
	SortOrder   *int       `json:"sort_order" validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

type Response struct {
	Category *Category `json:"category"`
}
