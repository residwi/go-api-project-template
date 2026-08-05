package review

import "github.com/google/uuid"

type CreateParams struct {
	OrderID uuid.UUID
	Rating  int
	Title   string
	Body    string
}
