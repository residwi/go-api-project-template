package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
	"github.com/residwi/go-api-project-template/internal/server/response"
)

type ProfileManager interface {
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, firstName, lastName string, phone *string) (*domain.User, error)
}

type Handler struct {
	service   ProfileManager
	validator *validator.Validator
}

func NewHandler(service ProfileManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	u, err := h.service.GetUser(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toUserResponse(u))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[updateProfileRequest](w, r, h.validator)
	if !ok {
		return
	}

	u, err := h.service.UpdateProfile(r.Context(), uc.UserID, req.FirstName, req.LastName, req.Phone)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toUserResponse(u))
}
