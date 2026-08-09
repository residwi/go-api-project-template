// Package user composes user's slices. It imports no transport package, so a
// worker or a future grpc server can construct this module without linking
// HTTP.
package user

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/user/adminupdate"
	adminupdatepg "github.com/residwi/go-api-project-template/internal/modules/user/adminupdate/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/credentials"
	credentialspg "github.com/residwi/go-api-project-template/internal/modules/user/credentials/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/user/query/postgres"
	queryredis "github.com/residwi/go-api-project-template/internal/modules/user/query/redis"
	"github.com/residwi/go-api-project-template/internal/modules/user/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/user/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/updateprofile"
	updateprofilepg "github.com/residwi/go-api-project-template/internal/modules/user/updateprofile/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/updaterole"
	updaterolepg "github.com/residwi/go-api-project-template/internal/modules/user/updaterole/postgres"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Cache  *redis.Client
	Logger *slog.Logger
}

// Module is Query, UpdateProfile, AdminUpdate, UpdateRole, Delete,
// Credentials. Credentials is exported because auth's login, register and
// refresh ports all need it -- the only slice consumed outside this module.
type Module struct {
	Query         *query.Reader
	UpdateProfile *updateprofile.Command
	AdminUpdate   *adminupdate.Command
	UpdateRole    *updaterole.Command
	Delete        *remove.Command
	Credentials   *credentials.Store
}

// New builds every slice. Cache may be nil: query's status cache degrades to
// query.NoCache rather than failing the boot.
func New(d Deps) *Module {
	var statusCache query.StatusCache = query.NoCache{}
	if d.Cache != nil {
		statusCache = queryredis.New(d.Cache)
	}

	return &Module{
		Query: query.New(querypg.New(d.Pool), statusCache, d.Logger),
		UpdateProfile: updateprofile.New(
			updateprofilepg.New(d.Pool),
		),
		// adminupdate, updaterole and remove each invalidate the cache through
		// their own narrow StatusInvalidator port; statusCache satisfies all
		// three by structural typing, so none of them imports query, its
		// sibling slice.
		AdminUpdate: adminupdate.New(adminupdatepg.New(d.Pool), statusCache, d.Logger),
		UpdateRole:  updaterole.New(updaterolepg.New(d.Pool), statusCache, d.Logger),
		Delete:      remove.New(removepg.New(d.Pool), statusCache, d.Logger),
		Credentials: credentials.New(credentialspg.New(d.Pool)),
	}
}
