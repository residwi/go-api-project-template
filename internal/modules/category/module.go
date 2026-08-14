package category

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/category/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/category/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/category/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/usecase/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/category/usecase/update/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

	Products remove.ProductCounter
}

type Module struct {
	Query  *query.UseCase
	Create *create.UseCase
	Update *update.UseCase
	Delete *remove.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool)),
		Create: create.New(createpg.New(d.Pool)),
		Update: update.New(updatepg.New(d.Pool)),
		Delete: remove.New(removepg.New(d.Pool), d.Products),
	}
}
