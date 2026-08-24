package http

type adminUpdateUserRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name"  validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone"      validate:"omitempty,max=20"`
	Active    *bool   `json:"active"`
}

type updateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}
