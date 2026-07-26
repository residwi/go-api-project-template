package notification

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type RouteDeps struct {
	Service *Service
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service}

	authed.HandleFunc("GET /notifications", h.List)
	authed.HandleFunc("PUT /notifications/{id}/read", h.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", h.MarkAllRead)
	authed.HandleFunc("GET /notifications/unread-count", h.UnreadCount)
}
