package shipping

type CreateShipmentRequest struct {
	Carrier        string `json:"carrier" validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

type UpdateTrackingRequest struct {
	Carrier        string `json:"carrier" validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}
