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

type Module struct {
	Query         *query.Reader
	UpdateProfile *updateprofile.Command
	AdminUpdate   *adminupdate.Command
	UpdateRole    *updaterole.Command
	Delete        *remove.Command
	Credentials   *credentials.Store
}

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
		AdminUpdate: adminupdate.New(adminupdatepg.New(d.Pool), statusCache, d.Logger),
		UpdateRole:  updaterole.New(updaterolepg.New(d.Pool), statusCache, d.Logger),
		Delete:      remove.New(removepg.New(d.Pool), statusCache, d.Logger),
		Credentials: credentials.New(credentialspg.New(d.Pool)),
	}
}
