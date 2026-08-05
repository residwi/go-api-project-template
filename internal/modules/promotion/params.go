package promotion

import "time"

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.
//
// Validate/Reserve are not given a params type: they already take plain
// (code string, orderAmount int64) arguments, not a request struct, so
// there is no dto-in-the-core cycle to break for the apply endpoint.

type CreateParams struct {
	Code           string
	Type           Type
	Value          int64
	MinOrderAmount int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       time.Time
	ExpiresAt      time.Time
	Active         bool
}

type UpdateParams struct {
	Code           string
	Type           Type
	Value          *int64
	MinOrderAmount *int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       *time.Time
	ExpiresAt      *time.Time
	Active         *bool
}

type ListParams struct {
	Page     int
	PageSize int
}
