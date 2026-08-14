package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	adjusthttp "github.com/residwi/go-api-project-template/internal/modules/inventory/adjust/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/query/http"
	restockhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/restock/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Inventory(admin *middleware.RouteGroup, m *inventory.Module, v *validator.Validator) {
	admin.HandleFunc("GET /inventory/{product_id}", queryhttp.New(m.Query).GetStock)
	admin.HandleFunc("PUT /inventory/{product_id}/restock", restockhttp.New(m.Restock, v).Restock)
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", adjusthttp.New(m.Adjust, v).Adjust)
}
