package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	markallreadhttp "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markallread/http"
	markreadhttp "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/markread/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/notification/usecase/query/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Notification(authed *middleware.RouteGroup, m *notification.Module) {
	query := queryhttp.New(m.Query)
	authed.HandleFunc("GET /notifications", query.List)
	authed.HandleFunc("GET /notifications/unread-count", query.UnreadCount)

	authed.HandleFunc("PUT /notifications/{id}/read", markreadhttp.New(m.MarkRead).MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", markallreadhttp.New(m.MarkAllRead).MarkAllRead)
}
