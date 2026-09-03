package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type UserManager interface {
	ListAdmin(ctx context.Context, params user.AdminListParams) ([]domain.User, int, error)
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	AdminUpdate(
		ctx context.Context, id uuid.UUID, firstName, lastName string, phone *string, active *bool,
	) (*domain.User, error)
	UpdateRole(ctx context.Context, requesterID, targetID uuid.UUID, role string) error
	Delete(ctx context.Context, requesterID, targetID uuid.UUID) error
}

type AdminHandler struct {
	service UserManager
}

func NewAdminHandler(service UserManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := user.AdminListParams{
		OffsetPage: page,
		Role:       r.URL.Query().Get("role"),
		Search:     r.URL.Query().Get("search"),
	}

	if activeStr := r.URL.Query().Get("active"); activeStr != "" {
		active := activeStr == "true"
		params.Active = &active
	}

	users, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminUserResponse, len(users))
	for i, u := range users {
		out[i] = toAdminUserResponse(&u)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	u, err := h.service.GetUser(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminUserResponse(u))
}

func (h *AdminHandler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[adminUpdateUserRequest](w, r)
	if !ok {
		return
	}

	u, err := h.service.AdminUpdate(r.Context(), id, req.FirstName, req.LastName, req.Phone, req.Active)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminUserResponse(u))
}

func (h *AdminHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := request.Bind[updateRoleRequest](w, r)
	if !ok {
		return
	}

	if err := h.service.UpdateRole(r.Context(), uc.UserID, id, req.Role); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
