package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	"github.com/residwi/go-api-project-template/internal/user"
)

type adminUpdateUserRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name" validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone" validate:"omitempty,max=20"`
	Active    *bool   `json:"active"`
}

func (r adminUpdateUserRequest) toAdminUpdateParams() user.AdminUpdateParams {
	return user.AdminUpdateParams{
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Phone:     r.Phone,
		Active:    r.Active,
	}
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[adminUpdateUserRequest](w, r, h.validator)
	if !ok {
		return
	}

	u, err := h.service.AdminUpdate(r.Context(), id, req.toAdminUpdateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminUserResponse(u))
}
