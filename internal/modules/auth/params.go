package auth

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
