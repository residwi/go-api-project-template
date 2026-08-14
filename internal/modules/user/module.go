package user

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/adminupdate"
	adminupdatepg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/adminupdate/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/credentials"
	credentialspg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/credentials/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/query/postgres"
	queryredis "github.com/residwi/go-api-project-template/internal/modules/user/usecase/query/redis"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/updateprofile"
	updateprofilepg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/updateprofile/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/updaterole"
	updaterolepg "github.com/residwi/go-api-project-template/internal/modules/user/usecase/updaterole/postgres"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Cache  *redis.Client
	Logger *slog.Logger
}

type Module struct {
	Query         *query.UseCase
	UpdateProfile *updateprofile.UseCase
	AdminUpdate   *adminupdate.UseCase
	UpdateRole    *updaterole.UseCase
	Delete        *remove.UseCase
	Credentials   *credentials.UseCase
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
