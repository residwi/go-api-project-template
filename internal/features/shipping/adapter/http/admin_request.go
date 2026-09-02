package http

type createShipmentRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

type updateTrackingRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}
