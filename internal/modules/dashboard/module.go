// Package dashboard composes dashboard's slices. It imports no transport
// package, so a worker or a future grpc server can construct this module
// without linking HTTP.
package dashboard

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/summary"
	summarypg "github.com/residwi/go-api-project-template/internal/modules/dashboard/summary/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool
}

type Module struct {
	Summary *summary.Reader
}

func New(d Deps) *Module {
	return &Module{
		Summary: summary.New(summarypg.New(d.Pool)),
	}
}
