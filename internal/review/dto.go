package review

import "github.com/google/uuid"

type CreateReviewRequest struct {
	OrderID uuid.UUID `json:"order_id" validate:"required"`
	Rating  int       `json:"rating" validate:"required,min=1,max=5"`
	Title   string    `json:"title" validate:"max=255"`
	Body    string    `json:"body"`
}
