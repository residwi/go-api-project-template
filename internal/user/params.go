package user

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.
//
// UpdateRoleParams and DeleteParams are not declared here -- they already
// exist in service.go (Phase 1 Task 9), named the actor and the subject to
// close a requester/target transposition hazard. This file only adds the
// two params types dto.go's request types used to satisfy.

type UpdateProfileParams struct {
	FirstName string
	LastName  string
	Phone     *string
}

type AdminUpdateParams struct {
	FirstName string
	LastName  string
	Phone     *string
	Active    *bool
}
