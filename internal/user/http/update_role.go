package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	"github.com/residwi/go-api-project-template/internal/user"
)

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
