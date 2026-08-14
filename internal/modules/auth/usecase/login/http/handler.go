package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	"github.com/residwi/go-api-project-template/internal/modules/auth/usecase/login"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Authenticator interface {
	Execute(ctx context.Context, p login.Params) (*domain.TokenPair, error)
}

type Handler struct {
	cmd       Authenticator
	validator *validator.Validator
}

func New(cmd Authenticator, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

type loginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (r loginRequest) toParams() login.Params {
	return login.Params{Email: r.Email, Password: r.Password}
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

	result, err := h.cmd.Execute(r.Context(), req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toTokenResponse(result))
}
