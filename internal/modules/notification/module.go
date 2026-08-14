package notification

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/notification/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/notification/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markallread"
	markallreadpg "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markallread/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markread"
	markreadpg "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markread/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/query/postgres"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

type Module struct {
	Query       *query.Reader
	MarkRead    *markread.Command
	MarkAllRead *markallread.Command
	Jobs        *jobs.Worker
}

func New(d Deps) *Module {
	return &Module{
		Query:       query.New(querypg.New(d.Pool)),
		MarkRead:    markread.New(markreadpg.New(d.Pool)),
		MarkAllRead: markallread.New(markallreadpg.New(d.Pool)),
		Jobs:        jobs.New(jobspg.New(d.Pool), d.Logger),
	}
}
