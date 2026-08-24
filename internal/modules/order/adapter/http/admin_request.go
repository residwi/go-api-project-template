package http

type updateStatusRequest struct {
	Status string `json:"status" validate:"required"`
}
