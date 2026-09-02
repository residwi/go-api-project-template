package http

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type AuthManager interface {
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	Register(ctx context.Context, email, password, firstName, lastName string) (*domain.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
}

type Handler struct {
	service AuthManager
}

func NewHandler(service AuthManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := request.Bind[loginRequest](w, r)
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
	req, ok := request.Bind[registerRequest](w, r)
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
	req, ok := request.Bind[refreshRequest](w, r)
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
