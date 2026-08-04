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

// userResponse is the public self-service shape. It deliberately omits role,
// active, and the timestamps: the client already learned its role from the
// auth token response, "active" is implied by being able to authenticate at
// all, and none of the three are the profile's own business to restate. The
// admin surface (adminUserResponse, admin_handler.go) legitimately carries all
// three -- that asymmetry is why the two types are not merged into one.
type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone,omitempty"`
}

// toUserResponse is the explicit mapping. PasswordHash, TokenVersion,
// DeletedAt, Role, Active, CreatedAt, and UpdatedAt are dropped: none of
// them are a field on userResponse, so there is no tag to strip and nothing
// to omit at serialization time -- they are structurally absent.
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
