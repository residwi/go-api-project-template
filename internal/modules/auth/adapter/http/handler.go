package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/response"
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

type loginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type registerRequest struct {
	Email     string `json:"email"      validate:"required,email"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type authUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"`
}

type tokenResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int              `json:"expires_in"`
	User         authUserResponse `json:"user"`
}

func toTokenResponse(tp *domain.TokenPair) tokenResponse {
	return tokenResponse{
		AccessToken:  tp.AccessToken,
		RefreshToken: tp.RefreshToken,
		ExpiresIn:    tp.ExpiresIn,
		User: authUserResponse{
			ID:        tp.User.ID,
			Email:     tp.User.Email,
			FirstName: tp.User.FirstName,
			LastName:  tp.User.LastName,
			Role:      tp.User.Role,
		},
	}
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
