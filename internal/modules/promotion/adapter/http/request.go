package http

type applyRequest struct {
	Code     string `json:"code"     validate:"required"`
	Subtotal int64  `json:"subtotal" validate:"required,min=1"`
}
