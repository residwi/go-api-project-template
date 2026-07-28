package http

import (
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type RouteDeps struct {
	Service *dashboard.Service
}

type handler struct {
	service *dashboard.Service
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service}

	admin.HandleFunc("GET /dashboard/summary", h.Summary)
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
}

// parseDateRange is shared by all three endpoints, not any one of them, so
// it lives here rather than being duplicated or arbitrarily owned by one
// endpoint file.
func parseDateRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		response.BadRequest(w, "from and to query parameters are required")
		return time.Time{}, time.Time{}, false
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		response.BadRequest(w, "invalid from date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}

	to, err = time.Parse("2006-01-02", toStr)
	if err != nil {
		response.BadRequest(w, "invalid to date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}

	// Set "to" to end of day
	to = to.Add(24*time.Hour - time.Nanosecond)

	return from, to, true
}
