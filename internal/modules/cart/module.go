package cart

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/add"
	addpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/add/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/clear"
	clearpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/clear/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/lock"
	lockpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/lock/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity"
	updatequantitypg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool     *pgxpool.Pool
	Tx       database.TxRunner
	MaxItems int
	Products ProductPorts
}

type ProductPorts interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*product.Info, error)
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]product.Info, error)
}

type Module struct {
	Query          *query.UseCase
	Add            *add.UseCase
	UpdateQuantity *updatequantity.UseCase
	Remove         *remove.UseCase
	Lock           *lock.UseCase
	ClearCart      *clear.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:          query.New(querypg.New(d.Pool), d.Products),
		Add:            add.New(addpg.New(d.Pool), d.Tx, d.Products, d.MaxItems),
		UpdateQuantity: updatequantity.New(updatequantitypg.New(d.Pool), d.Products),
		Remove:         remove.New(removepg.New(d.Pool)),
		Lock:           lock.New(lockpg.New(d.Pool)),
		ClearCart:      clear.New(clearpg.New(d.Pool)),
	}
}
