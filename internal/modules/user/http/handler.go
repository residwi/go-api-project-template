package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *user.Service
	validator *validator.Validator
}

// The public self-service shape. role came with the auth token, active is implied
// by authenticating at all, and the timestamps are not the profile's business.
// adminUserResponse carries all three, which is why the two are not merged.
type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone,omitempty"`
}

// PasswordHash, TokenVersion and DeletedAt are structurally absent, not omitted:
// there is no tag to strip.
func toUserResponse(u *user.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
	}
}

func (h *handler) Me(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	u, err := h.service.GetProfile(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toUserResponse(u))
}

type updateProfileRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name"  validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone"      validate:"omitempty,max=20"`
}

func (r updateProfileRequest) toUpdateProfileParams() user.UpdateProfileParams {
	return user.UpdateProfileParams{
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Phone:     r.Phone,
	}
}

func (h *handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
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
