package auth

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.

type RegisterParams struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

type LoginParams struct {
	Email    string
	Password string
}
