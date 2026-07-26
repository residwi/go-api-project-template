package user

type UpdateProfileRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name" validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone" validate:"omitempty,max=20"`
}

type AdminUpdateUserRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name" validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone" validate:"omitempty,max=20"`
	Active    *bool   `json:"active"`
}

type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

type ProfileResponse struct {
	User *User `json:"user"`
}
