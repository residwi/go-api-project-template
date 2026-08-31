package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	authhttp "github.com/residwi/go-api-project-template/internal/modules/auth/adapter/http"
	carthttp "github.com/residwi/go-api-project-template/internal/modules/cart/adapter/http"
	categoryhttp "github.com/residwi/go-api-project-template/internal/modules/category/adapter/http"
	checkouthttp "github.com/residwi/go-api-project-template/internal/modules/checkout/adapter/http"
	dashboardhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/adapter/http"
	inventoryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/adapter/http"
	notificationhttp "github.com/residwi/go-api-project-template/internal/modules/notification/adapter/http"
	orderhttp "github.com/residwi/go-api-project-template/internal/modules/order/adapter/http"
	paymenthttp "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/http"
	producthttp "github.com/residwi/go-api-project-template/internal/modules/product/adapter/http"
	promotionhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/adapter/http"
	reviewhttp "github.com/residwi/go-api-project-template/internal/modules/review/adapter/http"
	shippinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/adapter/http"
	userhttp "github.com/residwi/go-api-project-template/internal/modules/user/adapter/http"
	wishlisthttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
)

func NewRouter( //nolint:funlen // one flat wiring list: the middleware chain, the four route groups and all 65 routes in the order the router mounts them
	appCfg *config.Settings,
	modCfg bootstrap.Config,
	cache *redis.Client,
	logger *slog.Logger,
	app *bootstrap.App,
) http.Handler {
	mux := http.NewServeMux()

	router := web.NewRouter(mux)
	router.HandleFunc("GET /health", healthHandler())

	v := validator.New()

	authMW := authMiddleware(app.Auth, app.Users)
	adminMW := middleware.RequireRole("admin")

	api := router.Group("/api")
	authed := router.Group("/api", authMW)
	admin := authed.Group("/admin", adminMW)

	authLimiter := middleware.RateLimit(
		logger,
		cache,
		modCfg.Auth.RateLimit,
		modCfg.Auth.RateWindow,
	)
	authPublic := router.Group("/api", authLimiter)

	authHandler := authhttp.NewHandler(app.Auth, v)
	authPublic.HandleFunc("POST /auth/register", authHandler.Register)
	authPublic.HandleFunc("POST /auth/login", authHandler.Login)
	authPublic.HandleFunc("POST /auth/refresh", authHandler.Refresh)

	userHandler := userhttp.NewHandler(app.Users, v)
	authed.HandleFunc("GET /users/me", userHandler.Me)
	authed.HandleFunc("PUT /users/me", userHandler.Update)

	userAdminHandler := userhttp.NewAdminHandler(app.Users, v)
	admin.HandleFunc("GET /users", userAdminHandler.List)
	admin.HandleFunc("GET /users/{id}", userAdminHandler.Get)
	admin.HandleFunc("PUT /users/{id}", userAdminHandler.Update)
	admin.HandleFunc("PUT /users/{id}/role", userAdminHandler.UpdateRole)
	admin.HandleFunc("DELETE /users/{id}", userAdminHandler.Delete)

	categoryHandler := categoryhttp.NewHandler(app.Categories)
	api.HandleFunc("GET /categories", categoryHandler.List)
	api.HandleFunc("GET /categories/{slug}", categoryHandler.GetBySlug)

	categoryAdminHandler := categoryhttp.NewAdminHandler(app.Categories, v)
	admin.HandleFunc("POST /categories", categoryAdminHandler.Create)
	admin.HandleFunc("PUT /categories/{id}", categoryAdminHandler.Update)
	admin.HandleFunc("DELETE /categories/{id}", categoryAdminHandler.Delete)

	productHandler := producthttp.NewHandler(app.Products)
	api.HandleFunc("GET /products", productHandler.List)
	api.HandleFunc("GET /products/{slug}", productHandler.GetBySlug)

	productAdminHandler := producthttp.NewAdminHandler(app.Products, v)
	admin.HandleFunc("GET /products", productAdminHandler.List)
	admin.HandleFunc("GET /products/{id}", productAdminHandler.Get)
	admin.HandleFunc("POST /products", productAdminHandler.Create)
	admin.HandleFunc("PUT /products/{id}", productAdminHandler.Update)
	admin.HandleFunc("DELETE /products/{id}", productAdminHandler.Delete)

	inventoryHandler := inventoryhttp.NewHandler(app.Inventory, v)
	admin.HandleFunc("GET /inventory/{product_id}", inventoryHandler.GetStock)
	admin.HandleFunc("PUT /inventory/{product_id}/restock", inventoryHandler.Restock)
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", inventoryHandler.Adjust)

	cartHandler := carthttp.NewHandler(app.Carts, v)
	authed.HandleFunc("GET /cart", cartHandler.Get)
	authed.HandleFunc("POST /cart/items", cartHandler.Add)
	authed.HandleFunc("PUT /cart/items/{product_id}", cartHandler.Update)
	authed.HandleFunc("DELETE /cart/items/{product_id}", cartHandler.Remove)
	authed.HandleFunc("DELETE /cart", cartHandler.Clear)

	orderHandler := orderhttp.NewHandler(app.Orders)
	authed.HandleFunc("GET /orders", orderHandler.List)
	authed.HandleFunc("GET /orders/{id}", orderHandler.Get)

	orderAdminHandler := orderhttp.NewAdminHandler(app.Orders, v)
	admin.HandleFunc("GET /orders", orderAdminHandler.List)
	admin.HandleFunc("GET /orders/{id}", orderAdminHandler.Get)
	admin.HandleFunc("PUT /orders/{id}/status", orderAdminHandler.UpdateStatus)

	orderLimiter := middleware.RateLimit(
		logger,
		cache,
		modCfg.Order.RateLimit,
		modCfg.Order.RateWindow,
	)
	checkoutHandler := checkouthttp.NewHandler(app.Checkout, v)
	orderWrites := authed.Group("", orderLimiter)
	orderWrites.HandleFunc("POST /orders", checkoutHandler.Place)
	orderWrites.HandleFunc("POST /orders/{id}/pay", checkoutHandler.Retry)
	authed.HandleFunc("POST /orders/{id}/cancel", checkoutHandler.Cancel)

	api.HandleFunc("POST /payments/webhook", paymenthttp.NewWebhookHandler(app.Payments, logger).HandleWebhook)

	paymentAdminHandler := paymenthttp.NewAdminHandler(app.Payments)
	admin.HandleFunc("GET /payments", paymentAdminHandler.List)
	admin.HandleFunc("GET /payments/{id}", paymentAdminHandler.Get)
	admin.HandleFunc("POST /payments/{id}/refund", paymentAdminHandler.Refund)

	authed.HandleFunc("GET /orders/{id}/shipping", shippinghttp.NewHandler(app.Shipping).Get)

	shippingAdminHandler := shippinghttp.NewAdminHandler(app.Shipping, v)
	admin.HandleFunc("POST /orders/{id}/ship", shippingAdminHandler.Create)
	admin.HandleFunc("PUT /shipments/{id}/tracking", shippingAdminHandler.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", shippingAdminHandler.Deliver)

	reviewHandler := reviewhttp.NewHandler(app.Reviews, v)
	api.HandleFunc("GET /products/{id}/reviews", reviewHandler.List)
	authed.HandleFunc("POST /products/{id}/reviews", reviewHandler.Create)
	admin.HandleFunc("DELETE /reviews/{id}", reviewhttp.NewAdminHandler(app.Reviews).Delete)

	promotionHandler := promotionhttp.NewHandler(app.Promotions, v)
	authed.HandleFunc("POST /promotions/apply", promotionHandler.Apply)

	promotionAdminHandler := promotionhttp.NewAdminHandler(app.Promotions, v)
	admin.HandleFunc("POST /promotions", promotionAdminHandler.Create)
	admin.HandleFunc("GET /promotions", promotionAdminHandler.List)
	admin.HandleFunc("PUT /promotions/{id}", promotionAdminHandler.Update)
	admin.HandleFunc("DELETE /promotions/{id}", promotionAdminHandler.Delete)

	wishlistHandler := wishlisthttp.NewHandler(app.Wishlists, v)
	authed.HandleFunc("GET /wishlist", wishlistHandler.List)
	authed.HandleFunc("POST /wishlist/items", wishlistHandler.Add)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", wishlistHandler.Remove)

	notificationHandler := notificationhttp.NewHandler(app.Notifications)
	authed.HandleFunc("GET /notifications", notificationHandler.List)
	authed.HandleFunc("GET /notifications/unread-count", notificationHandler.UnreadCount)
	authed.HandleFunc("PUT /notifications/{id}/read", notificationHandler.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", notificationHandler.MarkAllRead)

	dashboardHandler := dashboardhttp.NewHandler(app.Dashboard)
	admin.HandleFunc("GET /dashboard/summary", dashboardHandler.Summary)
	admin.HandleFunc("GET /dashboard/top-products", dashboardHandler.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", dashboardHandler.Revenue)

	if appCfg.App.Env == "development" {
		mockgatewayserver.RegisterRoutes(
			mux,
			logger,
			mockgatewayserver.WithWebhookSecret(modCfg.Payment.WebhookSecret),
		)
	}

	return web.Chain(
		middleware.RequestID,
		middleware.Logging(logger),
		middleware.Recovery(logger),
		middleware.CORS(appCfg.CORS),
	)(mux)
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}
