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
	appCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading app config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(appCfg.Log.Level, appCfg.Log.Format)

	authCfg, cartCfg, orderCfg, paymentCfg, err := loadModuleConfigs(ctx, appCfg, appLog)
	if err != nil {
		return err
	}

	primaryDB, err := database.NewPrimaryPostgres(ctx, appCfg.Database)
	if err != nil {
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer primaryDB.Close()

	replicaDB, err := database.NewReplicaPostgres(ctx, appCfg.Database)
	if err != nil {
		if !errors.Is(err, apperror.ErrReplicaNotConfigured) {
			appLog.WarnContext(
				ctx,
				"failed to connect replica database, using primary",
				slog.String("error", err.Error()),
			)
		}
		replicaDB = nil
	}
	if replicaDB != nil {
		defer replicaDB.Close()
	}

	rdb, err := cache.NewRedis(ctx, appCfg.Redis)
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

	db := database.DB{Primary: primaryDB, Replica: replicaDB}

	app, err := bootstrap.New(authCfg, cartCfg, paymentCfg, db, rdb, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	handler := NewRouter(appCfg, authCfg, orderCfg, paymentCfg, rdb, appLog, app)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", appCfg.App.Port),
		Handler:      handler,
		ReadTimeout:  appCfg.App.ReadTimeout,
		WriteTimeout: appCfg.App.WriteTimeout,
		IdleTimeout:  appCfg.App.IdleTimeout,
		ErrorLog:     slog.NewLogLogger(appLog.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		appLog.InfoContext(
			ctx,
			"server starting",
			slog.Int("port", appCfg.App.Port),
			slog.String("env", appCfg.App.Env),
		)
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), appCfg.App.ShutdownTimeout)
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
	appCfg *config.Settings,
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

	orderCfg, err = order.LoadConfig(appCfg.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading order config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	paymentCfg, err = payment.LoadConfig(appCfg.App.Env, appCfg.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading payment config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	return authCfg, cartCfg, orderCfg, paymentCfg, nil
}

func NewRouter(
	appCfg *config.Settings,
	authCfg auth.Config,
	orderCfg order.Config,
	paymentCfg payment.Config,
	cache *redis.Client,
	logger *slog.Logger,
	app *bootstrap.App,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler())

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

	if appCfg.App.Env == "development" {
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
