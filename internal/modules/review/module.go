package review

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/review/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/review/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/review/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/review/usecase/remove/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

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
