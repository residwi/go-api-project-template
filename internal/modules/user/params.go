package user

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.
//
// UpdateRoleParams and DeleteParams are not declared here -- they live in
// service.go, where they name the actor and the subject separately to close a
// requester/target transposition hazard.

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
