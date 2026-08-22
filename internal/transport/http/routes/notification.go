package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationhttp "github.com/residwi/go-api-project-template/internal/modules/notification/adapter/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Notification(authed *middleware.RouteGroup, m *notification.Service) {
	h := notificationhttp.NewHandler(m)
	authed.HandleFunc("GET /notifications", h.List)
	authed.HandleFunc("GET /notifications/unread-count", h.UnreadCount)
	authed.HandleFunc("PUT /notifications/{id}/read", h.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", h.MarkAllRead)
}
