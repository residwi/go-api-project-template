package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/adminupdate"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type UserUpdater interface {
	Execute(ctx context.Context, id uuid.UUID, p adminupdate.Params) (*domain.User, error)
}

type Handler struct {
	usecase   UserUpdater
	validator *validator.Validator
}

func New(usecase UserUpdater, v *validator.Validator) *Handler {
	return &Handler{usecase: usecase, validator: v}
}

type adminUpdateUserRequest struct {
	FirstName string  `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  string  `json:"last_name"  validate:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone"      validate:"omitempty,max=20"`
	Active    *bool   `json:"active"`
}

func (r adminUpdateUserRequest) toParams() adminupdate.Params {
	return adminupdate.Params{
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Phone:     r.Phone,
		Active:    r.Active,
	}
}

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

func toAdminUserResponse(u *domain.User) adminUserResponse {
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

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[adminUpdateUserRequest](w, r, h.validator)
	if !ok {
		return
	}

	u, err := h.usecase.Execute(r.Context(), id, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminUserResponse(u))
}
