package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// refreshRequest has no params.go counterpart: auth.Service.RefreshToken
// already takes a plain string, not a request struct, so there is no
// dto-in-the-core cycle to break here. The wire type's only job is to carry
// the validate tag.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (h *handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[refreshRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toTokenResponse(result))
}
