package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	"github.com/residwi/go-api-project-template/internal/platform/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type AuthManager interface {
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	Register(ctx context.Context, email, password, firstName, lastName string) (*domain.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
}

type Handler struct {
	service   AuthManager
	validator *validator.Validator
}

func NewHandler(service AuthManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[loginRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toTokenResponse(result))
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[registerRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Register(r.Context(), req.Email, req.Password, req.FirstName, req.LastName)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toTokenResponse(result))
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[refreshRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toTokenResponse(result))
}
