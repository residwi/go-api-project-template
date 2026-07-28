package shipping

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport. UpdateTrackingParams keeps
// both fields -- UpdateTracking updates either independently when supplied
// non-empty, not just TrackingNumber.

type CreateParams struct {
	Carrier        string
	TrackingNumber string
}

type UpdateTrackingParams struct {
	Carrier        string
	TrackingNumber string
}
