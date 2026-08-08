// Package notification composes notification's slices. It imports no
// transport package, so a worker or a future grpc server can construct this
// module without linking HTTP. jobs/ owns the queue table, so it owns every
// operation on it: order/place enqueues through Jobs.EnqueueOrderPlaced, and
// cmd/worker drains it through Jobs itself, which satisfies both
// platform/jobs' Queue and Processor -- the reason notification still needs
// no worker/ package.
package notification

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/notification/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/notification/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/markallread"
	markallreadpg "github.com/residwi/go-api-project-template/internal/modules/notification/markallread/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/markread"
	markreadpg "github.com/residwi/go-api-project-template/internal/modules/notification/markread/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/notification/query/postgres"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Module is Query, MarkRead, MarkAllRead, Jobs. Jobs is exported because
// order/place's NotificationEnqueuer port and cmd/worker's jobs.Runner both
// need it -- the only slice consumed outside this module.
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
