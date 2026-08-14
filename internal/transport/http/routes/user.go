package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/user"
	adminupdatehttp "github.com/residwi/go-api-project-template/internal/modules/user/usecase/adminupdate/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/user/usecase/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/user/usecase/remove/http"
	updateprofilehttp "github.com/residwi/go-api-project-template/internal/modules/user/usecase/updateprofile/http"
	updaterolehttp "github.com/residwi/go-api-project-template/internal/modules/user/usecase/updaterole/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func User(authed, admin *middleware.RouteGroup, m *user.Module, v *validator.Validator) {
	authed.HandleFunc("GET /users/me", queryhttp.New(m.Query).Me)
	authed.HandleFunc("PUT /users/me", updateprofilehttp.New(m.UpdateProfile, v).Update)

	adminQuery := queryhttp.NewAdmin(m.Query)
	admin.HandleFunc("GET /users", adminQuery.List)
	admin.HandleFunc("GET /users/{id}", adminQuery.Get)

	admin.HandleFunc("PUT /users/{id}", adminupdatehttp.New(m.AdminUpdate, v).Update)
	admin.HandleFunc("PUT /users/{id}/role", updaterolehttp.New(m.UpdateRole, v).Update)
	admin.HandleFunc("DELETE /users/{id}", removehttp.New(m.Delete).Delete)
}
