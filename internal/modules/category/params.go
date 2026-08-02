package category

import "github.com/google/uuid"

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.

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
