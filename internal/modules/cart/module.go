// Package cart composes cart's slices. It imports no transport package, so a
// worker or a future grpc server can construct this module without linking
// HTTP.
package cart

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/add"
	addpg "github.com/residwi/go-api-project-template/internal/modules/cart/add/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/cart/empty"
	emptypg "github.com/residwi/go-api-project-template/internal/modules/cart/empty/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/lock"
	lockpg "github.com/residwi/go-api-project-template/internal/modules/cart/lock/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/cart/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/cart/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity"
	updatequantitypg "github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity/postgres"
	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool     *pgxpool.Pool
	Tx       database.TxRunner
	MaxItems int
	Products ProductPorts
}

// ProductPorts is the union of what cart's slices need from product. Each
// slice still declares its own narrow port; this exists so Deps has one field
// instead of one per slice.
type ProductPorts interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*productcontract.Product, error)
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error)
}

// Module is Query, Add, UpdateQuantity, Remove, Lock and Empty -- order and
// payment are not sliced yet (task 12/13), and order.CartProvider asks for
// LockCart, GetSnapshot and Clear as one whole-service port. That bundle is
// why Module itself exposes those three methods below: a single Go value can
// only satisfy an interface spanning three different slices by carrying the
// methods itself. The clear-the-cart slice is named empty, not clear: clear
// is a Go 1.21+ predeclared identifier, and "package clear" fails the same
// predeclared lint check that made cart's own item-delete slice "remove".
type Module struct {
	Query          *query.Reader
	Add            *add.Command
	UpdateQuantity *updatequantity.Command
	Remove         *remove.Command
	Lock           *lock.Command
	Empty          *empty.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:          query.New(querypg.New(d.Pool), d.Products),
		Add:            add.New(addpg.New(d.Pool), d.Tx, d.Products, d.MaxItems),
		UpdateQuantity: updatequantity.New(updatequantitypg.New(d.Pool), d.Products),
		Remove:         remove.New(removepg.New(d.Pool)),
		Lock:           lock.New(lockpg.New(d.Pool)),
		Empty:          empty.New(emptypg.New(d.Pool)),
	}
}

// LockCart is one of three delegators -- LockCart, GetSnapshot and Clear are
// the names order.CartProvider declares. Delegating them here lets bootstrap
// pass the whole Module for that port, unchanged from passing the old
// cart.Service, with no adapter written.
func (m *Module) LockCart(ctx context.Context, userID uuid.UUID) error {
	return m.Lock.LockCart(ctx, userID)
}

func (m *Module) GetSnapshot(ctx context.Context, userID uuid.UUID) (*contract.Cart, error) {
	return m.Query.GetSnapshot(ctx, userID)
}

func (m *Module) Clear(ctx context.Context, userID uuid.UUID) error {
	return m.Empty.Clear(ctx, userID)
}
