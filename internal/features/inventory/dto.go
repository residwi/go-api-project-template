package inventory

type RestockRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

type AdjustRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}
