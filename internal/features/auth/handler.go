package auth

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type Handler struct {
	service   *Service
	validator *validator.Validator
}

func NewHandler(service *Service, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
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
	var req LoginRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
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
	var req RefreshRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	result, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, result)
}
