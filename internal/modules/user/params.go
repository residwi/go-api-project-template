package user

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
