package http

type updateProfileRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name"  validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone"      validate:"omitempty,max=20"`
}
