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
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/routes"
)

func NewRouter(
	deps *Deps,
	app *bootstrap.App,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(deps.Logger, deps.Pool, deps.Cache))

	v := validator.New()

	authMiddleware := middleware.Auth(app.Auth.Token, app.Users.Query)
	adminMiddleware := middleware.RequireAdmin

	api := middleware.NewRouteGroup(mux, "/api")
	authed := middleware.NewRouteGroup(mux, "/api", authMiddleware)
	admin := middleware.NewRouteGroup(mux, "/api/admin", authMiddleware, adminMiddleware)

	authLimiter := middleware.RateLimit(
		deps.Logger,
		deps.Cache,
		deps.Auth.RateLimit,
		deps.Auth.RateWindow,
	)
	authPublic := middleware.NewRouteGroup(mux, "/api", authLimiter)

	orderWriteLimiter := middleware.RateLimit(
		deps.Logger,
		deps.Cache,
		deps.Order.RateLimit,
		deps.Order.RateWindow,
	)

	routes.Auth(authPublic, app.Auth, v)
	routes.User(authed, admin, app.Users, v)
	routes.Category(api, admin, app.Categories, v)
	routes.Product(api, admin, app.Products, v)
	routes.Inventory(admin, app.Inventory, v)
	routes.Cart(authed, app.Carts, v)
	routes.Order(authed, admin, app.Orders, v, orderWriteLimiter)
	routes.Payment(api, admin, app.Payments, deps.Logger)
	routes.Shipping(authed, admin, app.Shipping, v)
	routes.Review(api, authed, admin, app.Reviews, v)
	routes.Promotion(authed, admin, app.Promotions, v)
	routes.Wishlist(authed, app.Wishlists, v)
	routes.Notification(authed, app.Notifications)
	routes.Dashboard(admin, app.Dashboard)

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
