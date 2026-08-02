package http

// This package is the composition root: it is the one place that assembles
// every feature module into a single HTTP server, so it is also the one place
// that has to see all of them at once. Each feature names its adapters after
// their technology, not their feature — there are 14 packages called http and
// 13 called postgres under internal/<feature>/ — which is the right call
// inside a module (cart/postgres says what it is without stuttering
// "cartpostgres"). The cost of that choice is that every one of those imports
// needs a disambiguating alias here, following the <feature>http /
// <feature>pg convention. That is a deliberate trade, not accidental clutter:
// it pays the cost once, in this file, instead of in every module.
import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	authhttp "github.com/residwi/go-api-project-template/internal/modules/auth/http"
	carthttp "github.com/residwi/go-api-project-template/internal/modules/cart/http"
	cartpg "github.com/residwi/go-api-project-template/internal/modules/cart/postgres"
	categoryhttp "github.com/residwi/go-api-project-template/internal/modules/category/http"
	categorypg "github.com/residwi/go-api-project-template/internal/modules/category/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	dashboardhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/http"
	dashboardpg "github.com/residwi/go-api-project-template/internal/modules/dashboard/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventoryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/http"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationhttp "github.com/residwi/go-api-project-template/internal/modules/notification/http"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	orderhttp "github.com/residwi/go-api-project-template/internal/modules/order/http"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	paymenthttp "github.com/residwi/go-api-project-template/internal/modules/payment/http"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	producthttp "github.com/residwi/go-api-project-template/internal/modules/product/http"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/http"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/postgres"
	reviewhttp "github.com/residwi/go-api-project-template/internal/modules/review/http"
	reviewpg "github.com/residwi/go-api-project-template/internal/modules/review/postgres"
	shippinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/http"
	shippingpg "github.com/residwi/go-api-project-template/internal/modules/shipping/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	userhttp "github.com/residwi/go-api-project-template/internal/modules/user/http"
	userpg "github.com/residwi/go-api-project-template/internal/modules/user/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	wishlisthttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/http"
	wishlistpg "github.com/residwi/go-api-project-template/internal/modules/wishlist/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type Router struct {
	Handler    http.Handler
	PaymentSvc *payment.Service
}

func NewRouter(deps *Deps) *Router { //nolint:funlen // central route table: length is inherent to registering every feature's routes in one place
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(deps.Pool, deps.Redis))

	v := validator.New()

	userRepo := userpg.New(deps.Pool)
	categoryRepo := categorypg.New(deps.Pool)
	productRepo := productpg.New(deps.Pool)
	inventoryRepo := inventorypg.New(deps.Pool)
	cartRepo := cartpg.New(deps.Pool)
	orderRepo := orderpg.New(deps.Pool)
	paymentRepo := paymentpg.New(deps.Pool)
	shippingRepo := shippingpg.New(deps.Pool)
	reviewRepo := reviewpg.New(deps.Pool)
	promotionRepo := promotionpg.New(deps.Pool)
	wishlistRepo := wishlistpg.New(deps.Pool)
	notificationRepo := notificationpg.New(deps.Pool)
	dashboardRepo := dashboardpg.New(deps.Pool)

	txRunner := database.NewTxRunner(deps.Pool)

	userSvc := user.NewService(userRepo, deps.Redis)
	inventorySvc := inventory.NewService(inventoryRepo)
	productSvc := bootstrap.NewProductService(productRepo, inventorySvc)
	categorySvc := bootstrap.NewCategoryService(categoryRepo, productSvc)
	cartSvc := bootstrap.NewCartService(cartRepo, txRunner, productSvc, deps.Config.App.MaxCartItems)
	authSvc := auth.NewService(
		userSvc,
		deps.Config.JWT.Secret,
		deps.Config.JWT.Issuer,
		deps.Config.JWT.AccessTokenTTL,
		deps.Config.JWT.RefreshTokenTTL,
	)
	authSvc.SetBcryptCost(deps.Config.App.BcryptCost)
	promotionSvc := promotion.NewService(promotionRepo, txRunner)
	notificationSvc := notification.NewService(notificationRepo)
	wishlistSvc := wishlist.NewService(wishlistRepo)
	dashboardSvc := dashboard.NewService(dashboardRepo)

	orderSvc := bootstrap.NewOrderService(orderRepo, txRunner, cartSvc, inventorySvc, promotionSvc, notificationSvc)

	cfg := deps.Config
	gw := mockgateway.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	paymentSvc := bootstrap.NewPaymentService(paymentRepo, txRunner, gw, orderSvc, inventorySvc, promotionSvc)
	bootstrap.SetOrderPaymentDeps(orderSvc, paymentSvc)

	shippingSvc, shippingOrderProvider := bootstrap.NewShippingService(shippingRepo, txRunner, orderSvc)

	reviewSvc := bootstrap.NewReviewService(reviewRepo, orderSvc)

	tokenValidator := auth.NewTokenValidatorAdapter(authSvc)
	authMiddleware := middleware.Auth(tokenValidator, userSvc)
	adminMiddleware := middleware.RequireAdmin

	api := middleware.NewRouteGroup(mux, "/api")
	authed := middleware.NewRouteGroup(mux, "/api", authMiddleware)
	admin := middleware.NewRouteGroup(mux, "/api/admin", authMiddleware, adminMiddleware)

	// Auth endpoints run synchronous bcrypt and are unauthenticated, so they get
	// a dedicated per-IP rate limiter to blunt credential-stuffing / CPU exhaustion.
	authLimiter := middleware.RateLimit(deps.Redis, deps.Config.App.AuthRateLimit, deps.Config.App.AuthRateWindow)
	authPublic := middleware.NewRouteGroup(mux, "/api", authLimiter)

	// Throttle order placement/payment-retry (each runs a cart-lock + reserve +
	// charge); wired into order routes for the write endpoints only.
	orderWriteLimiter := middleware.RateLimit(deps.Redis, deps.Config.App.OrderRateLimit, deps.Config.App.OrderRateWindow)

	authhttp.RegisterRoutes(authPublic, authhttp.RouteDeps{Validator: v, Service: authSvc})
	userhttp.RegisterRoutes(authed, admin, userhttp.RouteDeps{Validator: v, Service: userSvc})
	categoryhttp.RegisterRoutes(api, admin, categoryhttp.RouteDeps{Validator: v, Service: categorySvc})
	producthttp.RegisterRoutes(api, admin, producthttp.RouteDeps{Validator: v, Service: productSvc})
	inventoryhttp.RegisterRoutes(admin, inventoryhttp.RouteDeps{Validator: v, Service: inventorySvc})
	carthttp.RegisterRoutes(authed, carthttp.RouteDeps{Validator: v, Service: cartSvc})
	orderhttp.RegisterRoutes(authed, admin, orderhttp.RouteDeps{Validator: v, Service: orderSvc, WriteLimiter: orderWriteLimiter})
	paymenthttp.RegisterRoutes(api, admin, paymenthttp.RouteDeps{Validator: v, Service: paymentSvc, WebhookSecret: cfg.Payment.WebhookSecret})
	shippinghttp.RegisterRoutes(authed, admin, shippinghttp.RouteDeps{Validator: v, Service: shippingSvc, Orders: shippingOrderProvider})
	reviewhttp.RegisterRoutes(api, authed, admin, reviewhttp.RouteDeps{Validator: v, Service: reviewSvc})
	promotionhttp.RegisterRoutes(authed, admin, promotionhttp.RouteDeps{Validator: v, Service: promotionSvc})
	wishlisthttp.RegisterRoutes(authed, wishlisthttp.RouteDeps{Validator: v, Service: wishlistSvc})
	notificationhttp.RegisterRoutes(authed, notificationhttp.RouteDeps{Service: notificationSvc})
	dashboardhttp.RegisterRoutes(admin, dashboardhttp.RouteDeps{Service: dashboardSvc})

	if deps.Config.App.Env == "development" {
		mockgatewayserver.RegisterRoutes(mux, mockgatewayserver.WithWebhookSecret(cfg.Payment.WebhookSecret))
	}

	return &Router{
		Handler: middleware.Chain(
			middleware.RequestID,
			middleware.Logging,
			middleware.Recovery,
			middleware.CORS(deps.Config.CORS),
		)(mux),
		PaymentSvc: paymentSvc,
	}
}

func healthHandler(pool *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		httpStatus := http.StatusOK
		details := make(map[string]string)

		if err := pool.Ping(r.Context()); err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			details["postgres"] = "down"
			slog.ErrorContext(r.Context(), "health check: postgres down", "error", err)
		} else {
			details["postgres"] = "up"
		}

		checkRedis(r.Context(), rdb, &status, &httpStatus, details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  status,
			"details": details,
		})
	}
}

func checkRedis(ctx context.Context, rdb *redis.Client, status *string, httpStatus *int, details map[string]string) {
	if rdb == nil {
		details["redis"] = "not configured"
		return
	}

	if err := rdb.Ping(ctx).Err(); err != nil {
		if *status == "healthy" {
			*status = "degraded"
			*httpStatus = http.StatusServiceUnavailable
		}
		details["redis"] = "down"
		slog.WarnContext(ctx, "health check: redis down", "error", err)
		return
	}

	details["redis"] = "up"
}
