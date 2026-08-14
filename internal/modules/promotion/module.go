package promotion

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/apply"
	applypg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/apply/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/reserve"
	reservepg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/reserve/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/update/postgres"
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
