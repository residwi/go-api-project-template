package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/user"
	adminupdatehttp "github.com/residwi/go-api-project-template/internal/modules/user/adminupdate/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/user/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/user/remove/http"
	updateprofilehttp "github.com/residwi/go-api-project-template/internal/modules/user/updateprofile/http"
	updaterolehttp "github.com/residwi/go-api-project-template/internal/modules/user/updaterole/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *user.Module
}

func RegisterRoutes(authed, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	queryhttp.NewAdmin(deps.Module.Query).RegisterHTTP(admin)

	updateprofilehttp.New(deps.Module.UpdateProfile, deps.Validator).RegisterHTTP(authed)

	adminupdatehttp.New(deps.Module.AdminUpdate, deps.Validator).RegisterHTTP(admin)
	updaterolehttp.New(deps.Module.UpdateRole, deps.Validator).RegisterHTTP(admin)
	removehttp.New(deps.Module.Delete).RegisterHTTP(admin)
}
