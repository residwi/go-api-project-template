package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return RunContext(ctx)
}

func RunContext(ctx context.Context) error {
	infra, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading infra config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(infra.Log.Level, infra.Log.Format)

	authCfg, cartCfg, orderCfg, paymentCfg, err := loadModuleConfigs(ctx, infra, appLog)
	if err != nil {
		return err
	}

	pool, err := database.NewPrimaryPostgres(ctx, infra.Database)
	if err != nil {
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	replicaPool, err := database.NewReplicaPostgres(ctx, infra.Database)
	if err != nil {
		if !errors.Is(err, apperror.ErrReplicaNotConfigured) {
			appLog.WarnContext(
				ctx,
				"failed to connect replica database, using primary",
				slog.String("error", err.Error()),
			)
		}
		replicaPool = nil
	}
	if replicaPool != nil {
		defer replicaPool.Close()
	}

	rdb, err := cache.NewRedis(ctx, infra.Redis)
	if err != nil {
		appLog.WarnContext(
			ctx,
			"failed to connect to redis, continuing without cache/rate-limiting",
			slog.String("error", err.Error()),
		)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	db := database.DB{Primary: pool, Replica: replicaPool}

	app, err := bootstrap.New(authCfg, cartCfg, paymentCfg, db, rdb, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	handler := NewRouter(infra, authCfg, orderCfg, paymentCfg, db, rdb, appLog, app)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", infra.App.Port),
		Handler:      handler,
		ReadTimeout:  infra.App.ReadTimeout,
		WriteTimeout: infra.App.WriteTimeout,
		IdleTimeout:  infra.App.IdleTimeout,
		ErrorLog:     slog.NewLogLogger(appLog.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		appLog.InfoContext(ctx, "server starting", slog.Int("port", infra.App.Port), slog.String("env", infra.App.Env))
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			appLog.ErrorContext(ctx, "server failed to start", slog.String("error", err.Error()))
			return fmt.Errorf("starting server: %w", err)
		}
	case <-ctx.Done():
	}

	appLog.InfoContext(ctx, "shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), infra.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLog.ErrorContext(ctx, "server shutdown failed", slog.String("error", err.Error()))
		return fmt.Errorf("server shutdown: %w", err)
	}

	appLog.InfoContext(ctx, "server stopped gracefully")
	return nil
}

func loadModuleConfigs(
	ctx context.Context,
	infra *config.Settings,
	appLog *slog.Logger,
) (auth.Config, cart.Config, order.Config, payment.Config, error) {
	var cartCfg cart.Config
	var orderCfg order.Config
	var paymentCfg payment.Config

	authCfg, err := auth.LoadConfig()
	if err != nil {
		appLog.ErrorContext(ctx, "loading auth config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	cartCfg, err = cart.LoadConfig()
	if err != nil {
		appLog.ErrorContext(ctx, "loading cart config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	orderCfg, err = order.LoadConfig(infra.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading order config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	paymentCfg, err = payment.LoadConfig(infra.App.Env, infra.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading payment config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	return authCfg, cartCfg, orderCfg, paymentCfg, nil
}

func NewRouter(
	infra *config.Settings,
	authCfg auth.Config,
	orderCfg order.Config,
	paymentCfg payment.Config,
	db database.DB,
	cache *redis.Client,
	logger *slog.Logger,
	app *bootstrap.App,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(logger, db, cache))

	v := validator.New()

	authMiddleware := middleware.Auth(app.Auth, app.Users)
	adminMiddleware := middleware.RequireAdmin

	api := middleware.NewRouteGroup(mux, "/api")
	authed := middleware.NewRouteGroup(mux, "/api", authMiddleware)
	admin := middleware.NewRouteGroup(mux, "/api/admin", authMiddleware, adminMiddleware)

	authLimiter := middleware.RateLimit(
		logger,
		cache,
		authCfg.RateLimit,
		authCfg.RateWindow,
	)
	authPublic := middleware.NewRouteGroup(mux, "/api", authLimiter)

	orderWriteLimiter := middleware.RateLimit(
		logger,
		cache,
		orderCfg.RateLimit,
		orderCfg.RateWindow,
	)

	registerRoutes(app, v, logger, api, authed, admin, authPublic, orderWriteLimiter)

	if infra.App.Env == "development" {
		mockgatewayserver.RegisterRoutes(
			mux,
			logger,
			mockgatewayserver.WithWebhookSecret(paymentCfg.WebhookSecret),
		)
	}

	return middleware.Chain(
		middleware.RequestID,
		middleware.Logging(logger),
		middleware.Recovery(logger),
		middleware.CORS(infra.CORS),
	)(mux)
}

func healthHandler(log *slog.Logger, db database.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		httpStatus := http.StatusOK
		details := make(map[string]string)

		if err := db.Primary.Ping(r.Context()); err != nil {
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
