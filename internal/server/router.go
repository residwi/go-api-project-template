package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/app"
	"github.com/residwi/go-api-project-template/internal/config"
	authhttp "github.com/residwi/go-api-project-template/internal/features/auth/adapter/http"
	carthttp "github.com/residwi/go-api-project-template/internal/features/cart/adapter/http"
	categoryhttp "github.com/residwi/go-api-project-template/internal/features/category/adapter/http"
	checkouthttp "github.com/residwi/go-api-project-template/internal/features/checkout/adapter/http"
	dashboardhttp "github.com/residwi/go-api-project-template/internal/features/dashboard/adapter/http"
	inventoryhttp "github.com/residwi/go-api-project-template/internal/features/inventory/adapter/http"
	notificationhttp "github.com/residwi/go-api-project-template/internal/features/notification/adapter/http"
	orderhttp "github.com/residwi/go-api-project-template/internal/features/order/adapter/http"
	paymenthttp "github.com/residwi/go-api-project-template/internal/features/payment/adapter/http"
	producthttp "github.com/residwi/go-api-project-template/internal/features/product/adapter/http"
	promotionhttp "github.com/residwi/go-api-project-template/internal/features/promotion/adapter/http"
	reviewhttp "github.com/residwi/go-api-project-template/internal/features/review/adapter/http"
	shippinghttp "github.com/residwi/go-api-project-template/internal/features/shipping/adapter/http"
	userhttp "github.com/residwi/go-api-project-template/internal/features/user/adapter/http"
	wishlisthttp "github.com/residwi/go-api-project-template/internal/features/wishlist/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
)

func NewRouter( //nolint:funlen // one flat wiring list: the middleware chain, the four route groups and all routes in the order the router mounts them
	appCfg *config.Settings,
	modCfg app.Config,
	cache *redis.Client,
	logger *slog.Logger,
	deps *app.Services,
) http.Handler {
	mux := http.NewServeMux()

	router := web.NewRouter(mux)
	router.HandleFunc("GET /health", healthHandler())

	authMW := middleware.Auth(deps.Auth)
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

	authHandler := authhttp.NewHandler(deps.Auth)
	authPublic.HandleFunc("POST /auth/register", authHandler.Register)
	authPublic.HandleFunc("POST /auth/login", authHandler.Login)
	authPublic.HandleFunc("POST /auth/refresh", authHandler.Refresh)

	userHandler := userhttp.NewHandler(deps.Users)
	authed.HandleFunc("GET /users/me", userHandler.Me)
	authed.HandleFunc("PUT /users/me", userHandler.Update)

	userAdminHandler := userhttp.NewAdminHandler(deps.Users)
	admin.HandleFunc("GET /users", userAdminHandler.List)
	admin.HandleFunc("GET /users/{id}", userAdminHandler.Get)
	admin.HandleFunc("PUT /users/{id}", userAdminHandler.Update)
	admin.HandleFunc("PUT /users/{id}/role", userAdminHandler.UpdateRole)
	admin.HandleFunc("DELETE /users/{id}", userAdminHandler.Delete)

	categoryHandler := categoryhttp.NewHandler(deps.Categories)
	api.HandleFunc("GET /categories", categoryHandler.List)
	api.HandleFunc("GET /categories/{slug}", categoryHandler.GetBySlug)

	categoryAdminHandler := categoryhttp.NewAdminHandler(deps.Categories)
	admin.HandleFunc("POST /categories", categoryAdminHandler.Create)
	admin.HandleFunc("PUT /categories/{id}", categoryAdminHandler.Update)
	admin.HandleFunc("DELETE /categories/{id}", categoryAdminHandler.Delete)

	productHandler := producthttp.NewHandler(deps.Products)
	api.HandleFunc("GET /products", productHandler.List)
	api.HandleFunc("GET /products/{slug}", productHandler.GetBySlug)

	productAdminHandler := producthttp.NewAdminHandler(deps.Products)
	admin.HandleFunc("GET /products", productAdminHandler.List)
	admin.HandleFunc("GET /products/{id}", productAdminHandler.Get)
	admin.HandleFunc("POST /products", productAdminHandler.Create)
	admin.HandleFunc("PUT /products/{id}", productAdminHandler.Update)
	admin.HandleFunc("DELETE /products/{id}", productAdminHandler.Delete)

	inventoryHandler := inventoryhttp.NewHandler(deps.Inventory)
	admin.HandleFunc("GET /inventory/{product_id}", inventoryHandler.GetStock)
	admin.HandleFunc("PUT /inventory/{product_id}/restock", inventoryHandler.Restock)
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", inventoryHandler.Adjust)

	cartHandler := carthttp.NewHandler(deps.Carts)
	authed.HandleFunc("GET /cart", cartHandler.Get)
	authed.HandleFunc("POST /cart/items", cartHandler.Add)
	authed.HandleFunc("PUT /cart/items/{product_id}", cartHandler.Update)
	authed.HandleFunc("DELETE /cart/items/{product_id}", cartHandler.Remove)
	authed.HandleFunc("DELETE /cart", cartHandler.Clear)

	orderHandler := orderhttp.NewHandler(deps.Orders)
	authed.HandleFunc("GET /orders", orderHandler.List)
	authed.HandleFunc("GET /orders/{id}", orderHandler.Get)

	orderAdminHandler := orderhttp.NewAdminHandler(deps.Orders)
	admin.HandleFunc("GET /orders", orderAdminHandler.List)
	admin.HandleFunc("GET /orders/{id}", orderAdminHandler.Get)
	admin.HandleFunc("PUT /orders/{id}/status", orderAdminHandler.UpdateStatus)

	orderLimiter := middleware.RateLimit(
		logger,
		cache,
		modCfg.Order.RateLimit,
		modCfg.Order.RateWindow,
	)
	checkoutHandler := checkouthttp.NewHandler(deps.Checkout)
	orderWrites := authed.Group("", orderLimiter)
	orderWrites.HandleFunc("POST /checkout", checkoutHandler.Checkout)
	orderWrites.HandleFunc("POST /orders/{id}/pay", checkoutHandler.Retry)
	authed.HandleFunc("POST /orders/{id}/cancel", checkoutHandler.Cancel)

	api.HandleFunc("POST /payments/webhook", paymenthttp.NewWebhookHandler(deps.Payments, logger).HandleWebhook)

	paymentAdminHandler := paymenthttp.NewAdminHandler(deps.Payments)
	admin.HandleFunc("GET /payments", paymentAdminHandler.List)
	admin.HandleFunc("GET /payments/{id}", paymentAdminHandler.Get)
	admin.HandleFunc("POST /payments/{id}/refund", paymentAdminHandler.Refund)

	authed.HandleFunc("GET /orders/{id}/shipping", shippinghttp.NewHandler(deps.Shipping).Get)

	shippingAdminHandler := shippinghttp.NewAdminHandler(deps.Shipping)
	admin.HandleFunc("POST /orders/{id}/ship", shippingAdminHandler.Create)
	admin.HandleFunc("PUT /shipments/{id}/tracking", shippingAdminHandler.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", shippingAdminHandler.Deliver)

	reviewHandler := reviewhttp.NewHandler(deps.Reviews)
	api.HandleFunc("GET /products/{id}/reviews", reviewHandler.List)
	authed.HandleFunc("POST /products/{id}/reviews", reviewHandler.Create)
	admin.HandleFunc("DELETE /reviews/{id}", reviewhttp.NewAdminHandler(deps.Reviews).Delete)

	promotionHandler := promotionhttp.NewHandler(deps.Promotions)
	authed.HandleFunc("POST /promotions/apply", promotionHandler.Apply)

	promotionAdminHandler := promotionhttp.NewAdminHandler(deps.Promotions)
	admin.HandleFunc("POST /promotions", promotionAdminHandler.Create)
	admin.HandleFunc("GET /promotions", promotionAdminHandler.List)
	admin.HandleFunc("PUT /promotions/{id}", promotionAdminHandler.Update)
	admin.HandleFunc("DELETE /promotions/{id}", promotionAdminHandler.Delete)

	wishlistHandler := wishlisthttp.NewHandler(deps.Wishlists)
	authed.HandleFunc("GET /wishlist", wishlistHandler.List)
	authed.HandleFunc("POST /wishlist/items", wishlistHandler.Add)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", wishlistHandler.Remove)

	notificationHandler := notificationhttp.NewHandler(deps.Notifications)
	authed.HandleFunc("GET /notifications", notificationHandler.List)
	authed.HandleFunc("GET /notifications/unread-count", notificationHandler.UnreadCount)
	authed.HandleFunc("PUT /notifications/{id}/read", notificationHandler.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", notificationHandler.MarkAllRead)

	dashboardHandler := dashboardhttp.NewHandler(deps.Dashboard)
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
		middleware.CORS(middleware.CORSOptions{
			AllowedOrigins: appCfg.CORS.AllowedOrigins,
			AllowedMethods: appCfg.CORS.AllowedMethods,
			AllowedHeaders: appCfg.CORS.AllowedHeaders,
			MaxAge:         appCfg.CORS.MaxAge,
		}),
	)(mux)
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}
