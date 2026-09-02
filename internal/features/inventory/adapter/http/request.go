package http

type restockRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

type adjustRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}
