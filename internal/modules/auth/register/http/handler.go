package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	"github.com/residwi/go-api-project-template/internal/modules/auth/register"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// Registerer is what Handler needs from register.Command: register.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type Registerer interface {
	Execute(ctx context.Context, p register.Params) (*domain.TokenPair, error)
}

type Handler struct {
	cmd       Registerer
	validator *validator.Validator
}

func New(cmd Registerer, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(api *middleware.RouteGroup) {
	api.HandleFunc("POST /auth/register", h.register)
}

// Validation lives here, not in the core: a service called from a worker should
// not inherit HTTP's validation vocabulary.
type registerRequest struct {
	Email     string `json:"email"      validate:"required,email"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
}

func (r registerRequest) toParams() register.Params {
	return register.Params{
		Email:     r.Email,
		Password:  r.Password,
		FirstName: r.FirstName,
		LastName:  r.LastName,
	}
}

// Declared here, not shared with auth's other slices: each endpoint holds its
// own copy so one endpoint's new field cannot appear in another's response.
// Mapped explicitly from usercontract.User, which also carries Active and
// TokenVersion -- adding a field there does not publish it.
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

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[registerRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.cmd.Execute(r.Context(), req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toTokenResponse(result))
}
