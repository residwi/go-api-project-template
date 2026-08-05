package shipping

type CreateParams struct {
	Carrier        string
	TrackingNumber string
}

type UpdateTrackingParams struct {
	Carrier        string
	TrackingNumber string
}
