package category

import "github.com/google/uuid"

type CreateParams struct {
	Name        string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   *int
	Active      *bool
}

type UpdateParams struct {
	Name        *string
	Description *string
	ParentID    *uuid.UUID
	SortOrder   *int
	Active      *bool
}
