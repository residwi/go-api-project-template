package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service   *auth.Service
	validator *validator.Validator
}

// registerRequest carries the validation rules. They live here, not in the
// core: a service called from a worker should not inherit HTTP's validation
// vocabulary.
type registerRequest struct {
	Email     string `json:"email"      validate:"required,email"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
}

// toRegisterParams is the seam: HTTP's validation vocabulary stops here, and
// the service receives a plain input struct.
func (r registerRequest) toRegisterParams() auth.RegisterParams {
	return auth.RegisterParams{
		Email:     r.Email,
		Password:  r.Password,
		FirstName: r.FirstName,
		LastName:  r.LastName,
	}
}

// authUserResponse is the user shape embedded in a token response. It is
// mapped explicitly from auth.UserResult, which also carries Active and
// TokenVersion -- neither of which belongs on the wire. Adding a field to
// UserResult does not add it here; this function has to be edited first.
type authUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"`
}

// tokenResponse is the shared wire shape returned by register, login, and
// refresh. It is declared once here and reused by Login and RefreshToken
// below rather than duplicated, since all three endpoints expose the
// identical shape.
type tokenResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int              `json:"expires_in"`
	User         authUserResponse `json:"user"`
}

func toTokenResponse(tp *auth.TokenPair) tokenResponse {
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

func (h *handler) Register(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[registerRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.Register(r.Context(), req.toRegisterParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toTokenResponse(result))
}

type loginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
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
