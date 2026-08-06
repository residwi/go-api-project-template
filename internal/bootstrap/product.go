package bootstrap

import (
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/product"
)

func NewProductService(repo product.Repository, inventorySvc *inventory.Service) *product.Service {
	return product.NewService(repo, inventorySvc, inventorySvc)
}
