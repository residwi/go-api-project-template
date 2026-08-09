package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/user/updaterole"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// RoleUpdater is what Handler needs from updaterole.Command:
// updaterole.Command satisfies it directly, so nothing sits between them,
// and the mockery-generated mock is the other implementation, used in
// handler_test.go.
type RoleUpdater interface {
	Execute(ctx context.Context, p updaterole.Params) error
}

type Handler struct {
	cmd       RoleUpdater
	validator *validator.Validator
}

func New(cmd RoleUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("PUT /users/{id}/role", h.update)
}

type updateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
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

	if err := h.cmd.Execute(r.Context(), updaterole.Params{
		RequesterID: uc.UserID,
		TargetID:    id,
		Role:        req.Role,
	}); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
