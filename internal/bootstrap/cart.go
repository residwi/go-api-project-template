package bootstrap

import (
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// NewCartService used to wrap productSvc in an adapter that translated
// product.Product into cart.ProductInfo, including the deleted_at ->
// "unavailable" rule. That rule now lives in product.Service.GetInfoByIDs
// itself, so productSvc satisfies cart.ProductLookup directly and this is a
// plain forward -- kept only so router.go, router_test.go and test/e2e keep
// constructing cart the way they always have.
func NewCartService(
	repo cart.Repository,
	tx database.TxRunner,
	productSvc *product.Service,
	maxCartItems int,
) *cart.Service {
	return cart.NewService(repo, tx, productSvc, maxCartItems)
}
