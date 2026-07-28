package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/auth"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Handler struct {
	service   *auth.Service
	validator *validator.Validator
}

func NewHandler(service *auth.Service, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[auth.RegisterRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Register(r.Context(), req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, result)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[auth.LoginRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Login(r.Context(), req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, result)
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[auth.RefreshRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, result)
}
