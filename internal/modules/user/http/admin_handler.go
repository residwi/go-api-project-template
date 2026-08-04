package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *user.Service
	validator *validator.Validator
}

// adminUserResponse is the admin shape of a user. It legitimately carries
// role, active, and the timestamps -- fields the public userResponse
// (handler.go) must not -- because an operator managing accounts needs
// them and a self-service caller does not. PasswordHash, TokenVersion, and
// DeletedAt still never appear: they are auth/lifecycle internals, not
// operator-facing account data.
type adminUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone,omitempty"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toAdminUserResponse(u *user.User) adminUserResponse {
	return adminUserResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
		Role:      u.Role,
		Active:    u.Active,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := user.ListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Role:     r.URL.Query().Get("role"),
		Search:   r.URL.Query().Get("search"),
	}

	if activeStr := r.URL.Query().Get("active"); activeStr != "" {
		active := activeStr == "true"
		params.Active = &active
	}

	users, total, err := h.service.List(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminUserResponse, len(users))
	for i, u := range users {
		out[i] = toAdminUserResponse(&u)
	}

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	u, err := h.service.AdminGetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminUserResponse(u))
}

type adminUpdateUserRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name"  validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone"      validate:"omitempty,max=20"`
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

type updateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

func (h *adminHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[updateRoleRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateRole(r.Context(), user.UpdateRoleParams{
		RequesterID: uc.UserID,
		TargetID:    id,
		Role:        req.Role,
	}); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), user.DeleteParams{
		RequesterID: uc.UserID,
		TargetID:    id,
	}); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
