package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
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
	"github.com/residwi/go-api-project-template/internal/platform/database"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type Router struct {
	Handler    http.Handler
	PaymentSvc *payment.Service
}

func NewRouter(deps *Deps) *Router { //nolint:funlen
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", healthHandler(deps.Pool, deps.Redis))

	// Create validator
	v := validator.New()

	// ── Repositories ──────────────────────────────────────────────────────
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

	// ── Services (no cross-feature deps) ──────────────────────────────────
	userSvc := user.NewService(userRepo, deps.Pool, deps.Redis)
	categorySvc := category.NewService(categoryRepo, deps.Pool)
	productSvc := product.NewService(productRepo, deps.Pool)
	inventorySvc := inventory.NewService(inventoryRepo, deps.Pool)
	cartSvc := cart.NewService(
		cartRepo,
		deps.Pool,
		&productLookupAdapter{svc: productSvc},
		&stockCheckerAdapter{svc: inventorySvc},
		deps.Config.App.MaxCartItems,
	)
	authSvc := auth.NewService(
		userSvc,
		deps.Config.JWT.Secret,
		deps.Config.JWT.Issuer,
		deps.Config.JWT.AccessTokenTTL,
		deps.Config.JWT.RefreshTokenTTL,
	)
	promotionSvc := promotion.NewService(promotionRepo, deps.Pool)
	notificationSvc := notification.NewService(notificationRepo)
	wishlistSvc := wishlist.NewService(wishlistRepo, deps.Pool)
	dashboardSvc := dashboard.NewService(dashboardRepo)

	// ── Order service (created with nil payment deps — set below) ─────────
	orderSvc := order.NewService(
		orderRepo,
		deps.Pool,
		&cartProviderAdapter{svc: cartSvc},
		&inventoryReserverAdapter{svc: inventorySvc},
		nil, // payment — set after paymentSvc is created
		nil, // paymentCancel — set after paymentSvc is created
		&couponReserverAdapter{svc: promotionSvc},
		&notificationEnqueuerAdapter{svc: notificationSvc},
	)

	// ── Payment gateway ───────────────────────────────────────────────────
	cfg := deps.Config
	gw := mockgw.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	// ── Payment service ───────────────────────────────────────────────────
	paymentSvc := payment.NewService(
		paymentRepo,
		deps.Pool,
		gw,
		&orderUpdaterAdapter{svc: orderSvc},
		&orderGetterAdapter{svc: orderSvc},
		&orderItemsGetterAdapter{svc: orderSvc},
		&inventoryDeductorAdapter{svc: inventorySvc},
		&inventoryReleaserAdapter{svc: inventorySvc},
		&inventoryRestockerAdapter{svc: inventorySvc},
		&couponReleaserAdapter{svc: promotionSvc},
	)

	// ── Break circular dependency: set payment deps on order service ──────
	orderSvc.SetPaymentDeps(
		&paymentInitiatorAdapter{svc: paymentSvc},
		&paymentJobCancellerAdapter{svc: paymentSvc},
	)

	// ── Shipping service ──────────────────────────────────────────────────
	shippingOrderProvider := &shippingOrderProviderAdapter{svc: orderSvc}
	shippingSvc := shipping.NewService(
		shippingRepo,
		shippingOrderProvider,
		&shippingOrderUpdaterAdapter{svc: orderSvc},
	)

	// ── Review service ────────────────────────────────────────────────────
	reviewSvc := review.NewService(
		reviewRepo,
		&purchaseVerifierAdapter{pool: deps.Pool},
	)

	// ── Middleware ─────────────────────────────────────────────────────────
	tokenValidator := auth.NewTokenValidatorAdapter(authSvc)
	authMiddleware := middleware.Auth(tokenValidator, userSvc)
	adminMiddleware := middleware.RequireAdmin

	// ── Route groups ──────────────────────────────────────────────────────
	api := middleware.NewRouteGroup(mux, "/api")
	authed := middleware.NewRouteGroup(mux, "/api", authMiddleware)
	admin := middleware.NewRouteGroup(mux, "/api/admin", authMiddleware, adminMiddleware)

	// ── Register feature routes ───────────────────────────────────────────
	auth.RegisterRoutes(api, auth.RouteDeps{Validator: v, Service: authSvc})
	user.RegisterRoutes(authed, admin, user.RouteDeps{Validator: v, Service: userSvc})
	category.RegisterRoutes(api, admin, category.RouteDeps{Validator: v, Service: categorySvc})
	product.RegisterRoutes(api, admin, product.RouteDeps{Validator: v, Service: productSvc})
	inventory.RegisterRoutes(admin, inventory.RouteDeps{Validator: v, Service: inventorySvc})
	cart.RegisterRoutes(authed, cart.RouteDeps{Validator: v, Service: cartSvc})
	order.RegisterRoutes(authed, admin, order.RouteDeps{Validator: v, Service: orderSvc})
	payment.RegisterRoutes(api, admin, payment.RouteDeps{Validator: v, Service: paymentSvc})
	shipping.RegisterRoutes(authed, admin, shipping.RouteDeps{Validator: v, Service: shippingSvc, Orders: shippingOrderProvider})
	review.RegisterRoutes(api, authed, admin, review.RouteDeps{Validator: v, Service: reviewSvc})
	promotion.RegisterRoutes(authed, admin, promotion.RouteDeps{Validator: v, Service: promotionSvc})
	wishlist.RegisterRoutes(authed, wishlist.RouteDeps{Validator: v, Service: wishlistSvc})
	notification.RegisterRoutes(authed, notification.RouteDeps{Service: notificationSvc})
	dashboard.RegisterRoutes(admin, dashboard.RouteDeps{Service: dashboardSvc})

	// ── Mock payment routes (development only) ────────────────────────────
	if deps.Config.App.Env == "development" {
		mockgw.RegisterRoutes(mux)
	}

	// Global middleware
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

		// Check PostgreSQL
		if err := pool.Ping(r.Context()); err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			details["postgres"] = "down"
			slog.ErrorContext(r.Context(), "health check: postgres down", "error", err)
		} else {
			details["postgres"] = "up"
		}

		// Check Redis
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

// ── Existing adapters ────────────────────────────────────────────────────

type productLookupAdapter struct {
	svc *product.Service
}

func (a *productLookupAdapter) GetByID(ctx context.Context, id uuid.UUID) (*cart.ProductInfo, error) {
	p, err := a.svc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &cart.ProductInfo{
		ID:       p.ID,
		Name:     p.Name,
		Price:    p.Price,
		Currency: p.Currency,
		Status:   p.Status,
	}, nil
}

type stockCheckerAdapter struct {
	svc *inventory.Service
}

func (a *stockCheckerAdapter) GetStock(ctx context.Context, productID uuid.UUID) (cart.StockInfo, error) {
	stock, err := a.svc.GetStock(ctx, productID)
	if err != nil {
		return cart.StockInfo{}, err
	}
	return cart.StockInfo{Available: stock.Available}, nil
}

// ── order.CartProvider adapter ───────────────────────────────────────────

type cartProviderAdapter struct {
	svc *cart.Service
}

func (a *cartProviderAdapter) GetCart(ctx context.Context, userID uuid.UUID) (*order.CartSnapshot, error) {
	c, err := a.svc.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	snap := &order.CartSnapshot{ID: c.ID}
	for _, item := range c.Items {
		si := order.CartSnapshotItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Currency = item.Product.Currency
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}

func (a *cartProviderAdapter) Clear(ctx context.Context, userID uuid.UUID) error {
	return a.svc.Clear(ctx, userID)
}

// ── order.InventoryReserver adapter ──────────────────────────────────────

type inventoryReserverAdapter struct {
	svc *inventory.Service
}

func (a *inventoryReserverAdapter) Reserve(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Reserve(ctx, productID, qty)
	return err
}

func (a *inventoryReserverAdapter) Release(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Release(ctx, productID, qty)
	return err
}

// ── order.PaymentInitiator adapter ───────────────────────────────────────

type paymentInitiatorAdapter struct {
	svc *payment.Service
}

func (a *paymentInitiatorAdapter) InitiatePayment(ctx context.Context, params order.InitiatePaymentParams) (order.PaymentResult, error) {
	result, err := a.svc.InitiatePayment(ctx, payment.InitiatePaymentParams{
		OrderID:         params.OrderID,
		Amount:          params.Amount,
		Currency:        params.Currency,
		PaymentMethodID: params.PaymentMethodID,
	})
	if err != nil {
		return order.PaymentResult{}, err
	}
	return order.PaymentResult{
		PaymentID:  result.PaymentID,
		PaymentURL: result.PaymentURL,
		Charged:    result.Charged,
	}, nil
}

// ── order.PaymentJobCanceller adapter ────────────────────────────────────

type paymentJobCancellerAdapter struct {
	svc *payment.Service
}

func (a *paymentJobCancellerAdapter) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.CancelJobsByOrderID(ctx, orderID)
}

// ── payment.OrderUpdater adapter ─────────────────────────────────────────

type orderUpdaterAdapter struct {
	svc *order.Service
}

func (a *orderUpdaterAdapter) UpdateStatus(ctx context.Context, orderID uuid.UUID, fromStatuses []string, toStatus string) error {
	from := make([]order.Status, len(fromStatuses))
	for i, s := range fromStatuses {
		from[i] = order.Status(s)
	}
	return a.svc.UpdateStatusMulti(ctx, orderID, from, order.Status(toStatus))
}

// ── payment.OrderGetter adapter ──────────────────────────────────────────

type orderGetterAdapter struct {
	svc *order.Service
}

func (a *orderGetterAdapter) GetByID(ctx context.Context, orderID uuid.UUID) (payment.OrderSnapshot, error) {
	o, err := a.svc.AdminGetByID(ctx, orderID)
	if err != nil {
		return payment.OrderSnapshot{}, err
	}
	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}
	return payment.OrderSnapshot{
		TotalAmount: o.TotalAmount,
		Currency:    o.Currency,
		Status:      string(o.Status),
		CouponCode:  couponCode,
	}, nil
}

// ── payment.OrderItemsGetter adapter ─────────────────────────────────────

type orderItemsGetterAdapter struct {
	svc *order.Service
}

func (a *orderItemsGetterAdapter) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]payment.OrderItemDTO, error) {
	items, err := a.svc.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	result := make([]payment.OrderItemDTO, len(items))
	for i, item := range items {
		result[i] = payment.OrderItemDTO{ProductID: item.ProductID, Quantity: item.Quantity}
	}
	return result, nil
}

// ── payment.InventoryDeductor adapter ────────────────────────────────────

type inventoryDeductorAdapter struct {
	svc *inventory.Service
}

func (a *inventoryDeductorAdapter) Deduct(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Deduct(ctx, productID, qty)
	return err
}

// ── payment.InventoryReleaser adapter ────────────────────────────────────

type inventoryReleaserAdapter struct {
	svc *inventory.Service
}

func (a *inventoryReleaserAdapter) Release(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Release(ctx, productID, qty)
	return err
}

// ── payment.InventoryRestocker adapter ───────────────────────────────────

type inventoryRestockerAdapter struct {
	svc *inventory.Service
}

func (a *inventoryRestockerAdapter) Restock(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Restock(ctx, productID, qty)
	return err
}

// ── payment.CouponReleaser adapter ───────────────────────────────────────

type couponReleaserAdapter struct {
	svc *promotion.Service
}

func (a *couponReleaserAdapter) Release(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Release(ctx, orderID)
}

// ── order.CouponReserver adapter ─────────────────────────────────────────

type couponReserverAdapter struct {
	svc *promotion.Service
}

func (a *couponReserverAdapter) Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error) {
	return a.svc.Reserve(ctx, code, userID, orderID, orderSubtotal)
}

func (a *couponReserverAdapter) Release(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Release(ctx, orderID)
}

// ── order.NotificationEnqueuer adapter ───────────────────────────────────

type notificationEnqueuerAdapter struct {
	svc *notification.Service
}

func (a *notificationEnqueuerAdapter) EnqueueOrderPlaced(ctx context.Context, userID, orderID uuid.UUID) error {
	return a.svc.EnqueueOrderPlaced(ctx, userID, orderID)
}

// ── shipping.OrderProvider adapter ───────────────────────────────────────

type shippingOrderProviderAdapter struct {
	svc *order.Service
}

func (a *shippingOrderProviderAdapter) GetByID(ctx context.Context, orderID uuid.UUID) (shipping.OrderInfo, error) {
	o, err := a.svc.AdminGetByID(ctx, orderID)
	if err != nil {
		return shipping.OrderInfo{}, err
	}
	return shipping.OrderInfo{ID: o.ID, UserID: o.UserID, Status: string(o.Status)}, nil
}

// ── shipping.OrderUpdater adapter ────────────────────────────────────────

type shippingOrderUpdaterAdapter struct {
	svc *order.Service
}

func (a *shippingOrderUpdaterAdapter) UpdateStatus(ctx context.Context, orderID uuid.UUID, fromStatuses []string, toStatus string) error {
	from := make([]order.Status, len(fromStatuses))
	for i, s := range fromStatuses {
		from[i] = order.Status(s)
	}
	return a.svc.UpdateStatusMulti(ctx, orderID, from, order.Status(toStatus))
}

// ── review.PurchaseVerifier adapter ──────────────────────────────────────

type purchaseVerifierAdapter struct {
	pool *pgxpool.Pool
}

func (a *purchaseVerifierAdapter) HasDeliveredOrder(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	db := database.DB(ctx, a.pool)
	var exists bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM order_items oi JOIN orders o ON o.id = oi.order_id
		 WHERE o.user_id = $1 AND oi.product_id = $2 AND o.status = 'delivered' LIMIT 1)`,
		userID, productID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
