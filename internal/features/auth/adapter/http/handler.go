package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type AuthManager interface {
	Login(ctx context.Context, email, password string) (*auth.TokenPair, error)
	Register(ctx context.Context, email, password, firstName, lastName string) (*auth.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
}

type Handler struct {
	service AuthManager
}

func NewHandler(service AuthManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) error {
	req, err := request.Bind[loginRequest](w, r)
	if err != nil {
		return err
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		return err
	}

	response.OK(w, toTokenResponse(result))
	return nil
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) error {
	req, err := request.Bind[registerRequest](w, r)
	if err != nil {
		return err
	}

	result, err := h.service.Register(r.Context(), req.Email, req.Password, req.FirstName, req.LastName)
	if err != nil {
		return err
	}

	response.Created(w, toTokenResponse(result))
	return nil
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) error {
	req, err := request.Bind[refreshRequest](w, r)
	if err != nil {
		return err
	}

	result, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		return err
	}

	response.OK(w, toTokenResponse(result))
	return nil
}
