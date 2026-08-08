// Package category composes category's slices. It imports no transport
// package, so a worker or a future grpc server can construct this module
// without linking HTTP.
package category

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/category/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/category/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/category/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/category/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/category/update/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

	// Products is product's service. It satisfies remove's ProductCounter port
	// by name-match, so no adapter stands between them.
	Products remove.ProductCounter
}

type Module struct {
	Query  *query.Reader
	Create *create.Command
	Update *update.Command
	Delete *remove.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool)),
		Create: create.New(createpg.New(d.Pool)),
		Update: update.New(updatepg.New(d.Pool)),
		Delete: remove.New(removepg.New(d.Pool), d.Products),
	}
}
