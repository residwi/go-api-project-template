package user

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type publicHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *publicHandler) Me(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	user, err := h.service.GetProfile(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, user)
}

func (h *publicHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[UpdateProfileRequest](w, r, h.validator)
	if !ok {
		return
	}

	user, err := h.service.UpdateProfile(r.Context(), uc.UserID, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, user)
}
