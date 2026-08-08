// Package review composes review's slices. It imports no transport package,
// so a worker or a future grpc server can construct this module without
// linking HTTP.
package review

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/review/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/review/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/review/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/review/remove/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

	// Purchase is order's service. It satisfies create's PurchaseVerifier port
	// by name-match, so no adapter stands between them.
	Purchase create.PurchaseVerifier
}

type Module struct {
	Query  *query.Reader
	Create *create.Command
	Delete *remove.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool)),
		Create: create.New(createpg.New(d.Pool), d.Purchase),
		Delete: remove.New(removepg.New(d.Pool)),
	}
}
