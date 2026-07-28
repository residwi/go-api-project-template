package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	"github.com/residwi/go-api-project-template/internal/user"
)

type updateProfileRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name" validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone" validate:"omitempty,max=20"`
}

func (r updateProfileRequest) toUpdateProfileParams() user.UpdateProfileParams {
	return user.UpdateProfileParams{
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Phone:     r.Phone,
	}
}

func (h *publicHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[updateProfileRequest](w, r, h.validator)
	if !ok {
		return
	}

	u, err := h.service.UpdateProfile(r.Context(), uc.UserID, req.toUpdateProfileParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toUserResponse(u))
}
