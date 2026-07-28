package http

import (
	"github.com/residwi/go-api-project-template/internal/notification"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Service *notification.Service
}

type handler struct {
	service *notification.Service
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service}

	authed.HandleFunc("GET /notifications", h.List)
	authed.HandleFunc("PUT /notifications/{id}/read", h.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", h.MarkAllRead)
	authed.HandleFunc("GET /notifications/unread-count", h.UnreadCount)
}
