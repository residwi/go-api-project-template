package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	loginhttp "github.com/residwi/go-api-project-template/internal/modules/auth/usecase/login/http"
	refreshhttp "github.com/residwi/go-api-project-template/internal/modules/auth/usecase/refresh/http"
	registerhttp "github.com/residwi/go-api-project-template/internal/modules/auth/usecase/register/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Auth(api *middleware.RouteGroup, m *auth.Module, v *validator.Validator) {
	api.HandleFunc("POST /auth/register", registerhttp.New(m.Register, v).Register)
	api.HandleFunc("POST /auth/login", loginhttp.New(m.Login, v).Login)
	api.HandleFunc("POST /auth/refresh", refreshhttp.New(m.Refresh, v).Refresh)
}
