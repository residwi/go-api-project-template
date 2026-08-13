package promotion

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/apply"
	applypg "github.com/residwi/go-api-project-template/internal/modules/promotion/apply/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/promotion/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/promotion/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/promotion/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/reserve"
	reservepg "github.com/residwi/go-api-project-template/internal/modules/promotion/reserve/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/promotion/update/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool *pgxpool.Pool
	Tx   database.TxRunner
}

type Module struct {
	Query   *query.Reader
	Create  *create.Command
	Update  *update.Command
	Delete  *remove.Command
	Apply   *apply.Command
	Reserve *reserve.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:   query.New(querypg.New(d.Pool)),
		Create:  create.New(createpg.New(d.Pool)),
		Update:  update.New(updatepg.New(d.Pool)),
		Delete:  remove.New(removepg.New(d.Pool)),
		Apply:   apply.New(applypg.New(d.Pool)),
		Reserve: reserve.New(reservepg.New(d.Pool), d.Tx),
	}
}
