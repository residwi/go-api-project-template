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
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, firstName, lastName string, phone *string) (*domain.User, error)
}

type Handler struct {
	service ProfileManager
}

func NewHandler(service ProfileManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
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

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := request.Bind[updateProfileRequest](w, r)
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
