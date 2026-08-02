package review

import "github.com/google/uuid"

// CreateParams is the service's input contract. It carries no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto this.
type CreateParams struct {
	OrderID uuid.UUID
	Rating  int
	Title   string
	Body    string
}
