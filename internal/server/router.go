package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/category"
	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/features/review"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/wishlist"
	"github.com/residwi/go-api-project-template/internal/middleware"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/wiring"
)

type Router struct {
	Handler    http.Handler
	PaymentSvc *payment.Service
}

func NewRouter(deps *Deps) *Router { //nolint:funlen // central route table: length is inherent to registering every feature's routes in one place
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(deps.Pool, deps.Redis))

	v := validator.New()

	userRepo := user.NewPostgresRepository(deps.Pool)
	categoryRepo := category.NewPostgresRepository(deps.Pool)
	productRepo := product.NewPostgresRepository(deps.Pool)
	inventoryRepo := inventory.NewPostgresRepository(deps.Pool)
	cartRepo := cart.NewPostgresRepository(deps.Pool)
	orderRepo := order.NewPostgresRepository(deps.Pool)
	paymentRepo := payment.NewPostgresRepository(deps.Pool)
	shippingRepo := shipping.NewPostgresRepository(deps.Pool)
	reviewRepo := review.NewPostgresRepository(deps.Pool)
	promotionRepo := promotion.NewPostgresRepository(deps.Pool)
	wishlistRepo := wishlist.NewPostgresRepository(deps.Pool)
	notificationRepo := notification.NewPostgresRepository(deps.Pool)
	dashboardRepo := dashboard.NewPostgresRepository(deps.Pool)

	userSvc := user.NewService(userRepo, deps.Redis)
	categorySvc := category.NewService(categoryRepo)
	productSvc := product.NewService(productRepo)
	inventorySvc := inventory.NewService(inventoryRepo)
	cartSvc := wiring.NewCartService(cartRepo, deps.Pool, productSvc, deps.Config.App.MaxCartItems)
	authSvc := auth.NewService(
		userSvc,
		deps.Config.JWT.Secret,
		deps.Config.JWT.Issuer,
		deps.Config.JWT.AccessTokenTTL,
		deps.Config.JWT.RefreshTokenTTL,
	)
	authSvc.SetBcryptCost(deps.Config.App.BcryptCost)
	promotionSvc := promotion.NewService(promotionRepo, deps.Pool)
	notificationSvc := notification.NewService(notificationRepo)
	wishlistSvc := wishlist.NewService(wishlistRepo)
	dashboardSvc := dashboard.NewService(dashboardRepo)

	orderSvc := wiring.NewOrderService(orderRepo, deps.Pool, cartSvc, inventorySvc, promotionSvc, notificationSvc)

	cfg := deps.Config
	gw := mockgw.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	paymentSvc := wiring.NewPaymentService(paymentRepo, deps.Pool, gw, orderSvc, inventorySvc, promotionSvc)
	wiring.SetOrderPaymentDeps(orderSvc, paymentSvc)

	shippingSvc, shippingOrderProvider := wiring.NewShippingService(shippingRepo, deps.Pool, orderSvc)

	reviewSvc := review.NewService(reviewRepo, orderSvc)

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

	auth.RegisterRoutes(authPublic, auth.RouteDeps{Validator: v, Service: authSvc})
	user.RegisterRoutes(authed, admin, user.RouteDeps{Validator: v, Service: userSvc})
	category.RegisterRoutes(api, admin, category.RouteDeps{Validator: v, Service: categorySvc})
	product.RegisterRoutes(api, admin, product.RouteDeps{Validator: v, Service: productSvc})
	inventory.RegisterRoutes(admin, inventory.RouteDeps{Validator: v, Service: inventorySvc})
	cart.RegisterRoutes(authed, cart.RouteDeps{Validator: v, Service: cartSvc})
	order.RegisterRoutes(authed, admin, order.RouteDeps{Validator: v, Service: orderSvc})
	payment.RegisterRoutes(api, admin, payment.RouteDeps{Validator: v, Service: paymentSvc, WebhookSecret: cfg.Payment.WebhookSecret})
	shipping.RegisterRoutes(authed, admin, shipping.RouteDeps{Validator: v, Service: shippingSvc, Orders: shippingOrderProvider})
	review.RegisterRoutes(api, authed, admin, review.RouteDeps{Validator: v, Service: reviewSvc})
	promotion.RegisterRoutes(authed, admin, promotion.RouteDeps{Validator: v, Service: promotionSvc})
	wishlist.RegisterRoutes(authed, wishlist.RouteDeps{Validator: v, Service: wishlistSvc})
	notification.RegisterRoutes(authed, notification.RouteDeps{Service: notificationSvc})
	dashboard.RegisterRoutes(admin, dashboard.RouteDeps{Service: dashboardSvc})

	if deps.Config.App.Env == "development" {
		mockgw.RegisterRoutes(mux, mockgw.WithWebhookSecret(cfg.Payment.WebhookSecret))
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
