package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventoryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Inventory(admin *middleware.RouteGroup, s *inventory.Service, v *validator.Validator) {
	h := inventoryhttp.NewHandler(s, v)
	admin.HandleFunc("GET /inventory/{product_id}", h.GetStock)
	admin.HandleFunc("PUT /inventory/{product_id}/restock", h.Restock)
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", h.Adjust)
}
