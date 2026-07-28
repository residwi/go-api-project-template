package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/auth"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (r loginRequest) toLoginParams() auth.LoginParams {
	return auth.LoginParams{Email: r.Email, Password: r.Password}
}

func (h *handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[loginRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Login(r.Context(), req.toLoginParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toTokenResponse(result))
}
