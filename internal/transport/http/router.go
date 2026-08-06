package http

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
	categoryhttp "github.com/residwi/go-api-project-template/internal/modules/category/http"
	dashboardhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/http"
	inventoryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/http"
	notificationhttp "github.com/residwi/go-api-project-template/internal/modules/notification/http"
	orderhttp "github.com/residwi/go-api-project-template/internal/modules/order/http"
	paymenthttp "github.com/residwi/go-api-project-template/internal/modules/payment/http"
	producthttp "github.com/residwi/go-api-project-template/internal/modules/product/http"
	promotionhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/http"
	reviewhttp "github.com/residwi/go-api-project-template/internal/modules/review/http"
	shippinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/http"
	userhttp "github.com/residwi/go-api-project-template/internal/modules/user/http"
	wishlisthttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func NewRouter(
	deps *Deps,
	app *bootstrap.App,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(deps.Logger, deps.Pool, deps.Cache))

	v := validator.New()

	tokenValidator := auth.NewTokenValidatorAdapter(app.Auth)
	authMiddleware := middleware.Auth(tokenValidator, app.Users)
	adminMiddleware := middleware.RequireAdmin

	api := middleware.NewRouteGroup(mux, "/api")
	authed := middleware.NewRouteGroup(mux, "/api", authMiddleware)
	admin := middleware.NewRouteGroup(mux, "/api/admin", authMiddleware, adminMiddleware)

	// Auth endpoints run synchronous bcrypt and are unauthenticated, so they get
	// a dedicated per-IP rate limiter to blunt credential-stuffing / CPU exhaustion.
	authLimiter := middleware.RateLimit(
		deps.Logger,
		deps.Cache,
		deps.Auth.RateLimit,
		deps.Auth.RateWindow,
	)
	authPublic := middleware.NewRouteGroup(mux, "/api", authLimiter)

	// Placement and payment retry each run a cart-lock, a reserve and a charge.
	orderWriteLimiter := middleware.RateLimit(
		deps.Logger,
		deps.Cache,
		deps.Order.RateLimit,
		deps.Order.RateWindow,
	)

	authhttp.RegisterRoutes(authPublic, authhttp.RouteDeps{Validator: v, Service: app.Auth})
	userhttp.RegisterRoutes(authed, admin, userhttp.RouteDeps{Validator: v, Service: app.Users})
	categoryhttp.RegisterRoutes(api, admin, categoryhttp.RouteDeps{Validator: v, Service: app.Categories})
	producthttp.RegisterRoutes(api, admin, producthttp.RouteDeps{Validator: v, Service: app.Products})
	inventoryhttp.RegisterRoutes(admin, inventoryhttp.RouteDeps{Validator: v, Service: app.Inventory})
	carthttp.RegisterRoutes(authed, carthttp.RouteDeps{Validator: v, Service: app.Carts})
	orderhttp.RegisterRoutes(
		authed,
		admin,
		orderhttp.RouteDeps{Validator: v, Service: app.Orders, WriteLimiter: orderWriteLimiter},
	)
	paymenthttp.RegisterRoutes(
		api,
		admin,
		paymenthttp.RouteDeps{
			Validator:     v,
			Service:       app.Payments,
			WebhookSecret: deps.Payment.WebhookSecret,
			Logger:        deps.Logger,
		},
	)
	shippinghttp.RegisterRoutes(
		authed,
		admin,
		shippinghttp.RouteDeps{Validator: v, Service: app.Shipping, Orders: app.Orders},
	)
	reviewhttp.RegisterRoutes(api, authed, admin, reviewhttp.RouteDeps{Validator: v, Service: app.Reviews})
	promotionhttp.RegisterRoutes(authed, admin, promotionhttp.RouteDeps{Validator: v, Service: app.Promotions})
	wishlisthttp.RegisterRoutes(authed, wishlisthttp.RouteDeps{Validator: v, Service: app.Wishlists})
	notificationhttp.RegisterRoutes(authed, notificationhttp.RouteDeps{Service: app.Notifications})
	dashboardhttp.RegisterRoutes(admin, dashboardhttp.RouteDeps{Service: app.Dashboard})

	if deps.Infra.App.Env == "development" {
		mockgatewayserver.RegisterRoutes(
			mux,
			deps.Logger,
			mockgatewayserver.WithWebhookSecret(deps.Payment.WebhookSecret),
		)
	}

	return middleware.Chain(
		middleware.RequestID,
		middleware.Logging(deps.Logger),
		middleware.Recovery(deps.Logger),
		middleware.CORS(deps.Infra.CORS),
	)(mux)
}

func healthHandler(log *slog.Logger, pool *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		httpStatus := http.StatusOK
		details := make(map[string]string)

		if err := pool.Ping(r.Context()); err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			details["postgres"] = "down"
			log.ErrorContext(r.Context(), "health check: postgres down", slog.String("error", err.Error()))
		} else {
			details["postgres"] = "up"
		}

		checkRedis(r.Context(), log, rdb, &status, &httpStatus, details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  status,
			"details": details,
		})
	}
}

func checkRedis(
	ctx context.Context,
	log *slog.Logger,
	rdb *redis.Client,
	status *string,
	httpStatus *int,
	details map[string]string,
) {
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
		log.WarnContext(ctx, "health check: redis down", slog.String("error", err.Error()))
		return
	}

	details["redis"] = "up"
}
