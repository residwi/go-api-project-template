package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type ProfileManager interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, firstName, lastName string, phone *string) (*domain.User, error)
}

type Handler struct {
	service ProfileManager
}

func NewHandler(service ProfileManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	u, err := h.service.GetByID(r.Context(), uc.UserID)
	if err != nil {
		return err
	}

	response.OK(w, toUserResponse(u))
	return nil
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	req, err := request.Bind[updateProfileRequest](w, r)
	if err != nil {
		return err
	}

	u, err := h.service.UpdateProfile(r.Context(), uc.UserID, req.FirstName, req.LastName, req.Phone)
	if err != nil {
		return err
	}

	response.OK(w, toUserResponse(u))
	return nil
}
