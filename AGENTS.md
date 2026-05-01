# Project Overview

Production-ready ecommerce API template built in Go 1.26. It exposes RESTful endpoints for a complete ecommerce system (auth, users, categories, products, inventory, cart, orders, payments, shipping, reviews, promotions, wishlist, notifications, admin dashboard) and runs a separate payment job worker process. The data layer is PostgreSQL (pgx/v5) with Redis caching. The codebase follows Feature-Based Clean Architecture (vertical slicing) with strict dependency inversion.

## Repository Structure

- `cmd/api/` — API server binary entry point (`server.Run()`)
- `cmd/worker/` — Payment job worker binary entry point
- `internal/config/` — Configuration management (`godotenv` + `envconfig`)
- `internal/core/` — Shared value objects (Money, Address, Pagination, Slugify, AppError, Response helpers) — no feature deps
- `internal/features/` — 14 feature modules, each self-contained:
  - `auth/` — Authentication (register, login, JWT refresh)
  - `user/` — User management (profile, admin CRUD)
  - `category/` — Product categories (public list, admin CRUD)
  - `product/` — Product catalog (public list, admin CRUD)
  - `inventory/` — Stock management (admin only)
  - `cart/` — Shopping cart (authenticated users)
  - `order/` — Order lifecycle (place, pay, cancel, admin management)
  - `payment/` — Payment processing + worker jobs
  - `shipping/` — Shipment tracking (admin + user view)
  - `review/` — Product reviews (purchase-verified)
  - `promotion/` — Coupons/promotions (apply + admin CRUD)
  - `wishlist/` — User wishlists
  - `notification/` — User notifications
  - `dashboard/` — Admin analytics (summary, top products, revenue)
- `internal/middleware/` — HTTP middleware (auth, admin, CORS, logging, recovery, request ID, rate limiting)
- `internal/platform/` — Infrastructure layer:
  - `database/` — PostgreSQL connection + transaction helpers
  - `cache/` — Redis client
  - `payment/` — Payment gateway interface + implementations (mock, Stripe, Midtrans)
  - `validator/` — Request validation
  - `logger/` — Structured logging setup
  - `email/` — Email service (future)
  - `storage/` — File storage (future)
- `internal/server/` — HTTP server bootstrap, router, cross-feature adapter wiring
- `db/migrations/` — Timestamped SQL migration files (goose)
- `mocks/` — Generated mocks (mockery v3); sub-dirs per feature
- `bin/` — Build output

## Build & Development Commands

```bash
# Install dependencies
make setup

# Build all binaries (API + worker)
make build

# Build API server only
make build-api

# Build worker only
make build-worker

# Run API server
make run

# Run worker
make run-worker

# Run with hot reload (API)
make dev

# Run tests with race detector + coverage
make test

# Run tests and generate HTML coverage report
make test-coverage

# Remove shared test containers (postgres + redis)
make test-clean

# Lint
make lint

# Format
make fmt

# Vet
make vet

# Tidy modules
make tidy

# Generate mocks (mockery v3)
make mocks

# Database migrations
make migrate-up
make migrate-down
make migrate-down-all
make migrate-create name=migration_name
make migrate-status

# Docker
make docker-build
make docker-up
make docker-dev
make docker-down
make docker-logs
make docker-clean

# Full pipeline
make all   # fmt → vet → lint → test → build
make ci    # deps → fmt → vet → lint → test
```

## Code Style & Conventions

- Language: Go 1.26; use `net/http` ServeMux (no third-party routers).
- JSON: `encoding/json` — never `github.com/bytedance/sonic`.
- Validation: `go-playground/validator/v10`.
- Config: `godotenv` + `kelseyhightower/envconfig`; struct tags define env var names and defaults.
- Logging: `log/slog` only (structured, JSON format in production).
- Naming: packages are short, singular nouns (`user`, `product`, `cart`). Files inside a feature: `handler.go`, `service.go`, `repository.go`, `model.go`, `dto.go`, `routes.go`. Tests: `*_test.go` in the same package.
- Error handling: Application errors in `core/apperror.go`; use sentinel errors like `core.ErrNotFound`, `core.ErrBadRequest`, `core.ErrUnauthorized` for structured error responses. Wrap with `fmt.Errorf("%w: ...", core.ErrBadRequest)` for additional context.
- Response helpers: Use `response.OK()`, `response.Created()`, `response.BadRequest()`, `response.NotFound()`, etc. from `internal/core/response/`.
- Formatting: `gofmt -s`; enforced via `make fmt` and golangci-lint.
- Imports: Group as stdlib, blank line, third-party, blank line, local (`go-api-project-template/...`).
- Commit messages: imperative mood, e.g. "Add webhook signature validation". No conventional-commits prefix required.
- Comments: Add only when necessary. Explain why, not how.
- Duplication over wrong abstraction: Prefer duplicating code over introducing a shared abstraction that doesn't quite fit.
- Cross-feature dependencies: Use inline interfaces at the top of the consumer's `service.go` (dependency inversion). No shared `port.go` files. Each feature defines only the methods it needs from other features.

## Architecture Notes

```text
┌───────────────────────────────────────────────────────────┐
│               cmd/api (HTTP server binary)                │
│  server.Run() ──► Router ──► Middleware Chain              │
│                     │                                     │
│  ┌──────────────────┼──────────────────────────┐          │
│  │  internal/features (14 vertical slices)     │          │
│  │  ┌────────┐ ┌────────┐ ┌─────────┐  ...    │          │
│  │  │  auth  │ │  user  │ │ product │         │          │
│  │  │handler │ │handler │ │ handler │         │          │
│  │  └───┬────┘ └───┬────┘ └────┬────┘         │          │
│  │      │          │           │               │          │
│  │  ┌───▼────┐ ┌───▼────┐ ┌────▼────┐         │          │
│  │  │  auth  │ │  user  │ │ product │         │          │
│  │  │service │ │service │ │ service │         │          │
│  │  └───┬────┘ └───┬────┘ └────┬────┘         │          │
│  │      │          │           │               │          │
│  │      └──────────┼───────────┘               │          │
│  │                 │ (inline interfaces)        │          │
│  │  ┌──────────────▼───────────────────────┐   │          │
│  │  │      PostgreSQL repositories         │   │          │
│  │  │   (embedded in each feature pkg)     │   │          │
│  │  └──────────────┬───────────────────────┘   │          │
│  └─────────────────┼───────────────────────────┘          │
│                    │                                      │
│  ┌─────────────────▼───────────────────────────┐          │
│  │  internal/platform (infrastructure)         │          │
│  │  database │ cache │ payment │ validator │ …  │          │
│  └─────────────────────────────────────────────┘          │
│                    │                                      │
│  ┌─────────────────▼──────────────────────────┐           │
│  │  cmd/worker (payment job processor binary)  │           │
│  │  Polls payment_jobs table on interval       │           │
│  │  ──► paymentSvc ──► gateway ──► order update│           │
│  └─────────────────────────────────────────────┘           │
│                    │                                      │
│           PostgreSQL + Redis                              │
└───────────────────────────────────────────────────────────┘
```

**Data flow (place order):**

1. User sends POST to `/api/orders` with cart + payment method.
2. `order.Handler` validates request → `order.Service.PlaceOrder()` locks cart, snapshots items, reserves inventory, applies coupon (if any), creates order + items in a transaction.
3. `payment.Service.InitiatePayment()` creates a payment record, calls the payment gateway to get a payment URL or charge.
4. `payment.Worker` (in `cmd/worker`) polls `payment_jobs` on an interval, processes pending payments, updates order status on success/failure, deducts/releases inventory accordingly.

## Testing Strategy

- Framework: `testing` + `github.com/stretchr/testify` (assert, require). Always use `assert` for non-fatal checks and `require` when a failure should stop the test immediately (e.g. nil checks before dereferencing).
- Mocks: Generated with mockery v3 (config in `.mockery.yml`). Run `make mocks` to regenerate into `mocks/`. Use the mockery expecter API (`.EXPECT().Method().Return(...)`) — never set up mock calls manually.
- Unit tests live alongside source files (`*_test.go`). Every feature package has handler and service tests.
- Run all tests: `make test` (`go test -v -race -cover ./...`).
- Coverage report: `make test-coverage` → `coverage.html`.
- CI pipeline: `make ci` (deps → fmt → vet → lint → test).

### Testing Rules for Agents

1. **Test behavior, not implementation.** Each test should verify a user-visible outcome (returned value, error, or side effect), not internal wiring. This enables refactoring and optimisation without
   breaking tests.

2. **Use stretchr/testify for all assertions.** Use `assert` for soft checks, `require` when the test cannot continue without the value.

3. **Use mockery-generated mocks from `mocks/`.** Set expectations with the expecter API:

   ```go
   // ✅ Correct — expecter API
   repo := mocks.NewMockRepository(t)
   repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

   // ❌ Wrong — manual On/Return
   repo.On("GetByID", mock.Anything, orderID).Return(existingOrder, nil)
   ```

4. **Prefer subtests over table-driven tests.** Each subtest should cover one logical scenario with a descriptive name:

   ```go
   // ✅ Correct — separate subtests, each with its own setup
   func TestService_RetryPayment(t *testing.T) {
       t.Run("success", func(t *testing.T) {
           svc, repo, _, _, payment, _, _, _ := newTestService(t)
           existingOrder := &order.Order{
               ID: orderID, UserID: userID,
               Status: order.StatusAwaitingPayment, TotalAmount: 5000,
           }
           repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
           payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).
               Return(order.PaymentResult{PaymentID: uuid.New()}, nil)

           result, err := svc.RetryPayment(ctx, userID, orderID, "pm_test")
           require.NoError(t, err)
           assert.NotNil(t, result)
       })

       t.Run("not payable when status is paid", func(t *testing.T) {
           svc, repo, _, _, _, _, _, _ := newTestService(t)
           existingOrder := &order.Order{
               ID: orderID, UserID: userID, Status: order.StatusPaid,
           }
           repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

           result, err := svc.RetryPayment(ctx, userID, orderID, "pm_test")
           assert.Nil(t, result)
           assert.ErrorIs(t, err, core.ErrOrderNotPayable)
       })
   }

   // ❌ Wrong — table-driven test with complex struct
   func TestService_RetryPayment(t *testing.T) {
       tests := []struct {
           name    string
           order   *order.Order
           wantErr error
       }{
           {"success", &order.Order{Status: order.StatusAwaitingPayment}, nil},
           {"not payable", &order.Order{Status: order.StatusPaid}, core.ErrOrderNotPayable},
       }
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) { /* ... */ })
       }
   }
   ```

5. **No monolithic tests.** Break large scenarios into focused subtests:

   ```go
   // ✅ Correct — one concern per test
   func TestService_CancelOrder(t *testing.T) {
       t.Run("not found", func(t *testing.T) { /* ... */ })
       t.Run("not owned by user", func(t *testing.T) { /* ... */ })
       t.Run("payment processing returns ErrOrderCharging", func(t *testing.T) { /* ... */ })
       t.Run("invalid transition from delivered", func(t *testing.T) { /* ... */ })
   }

   // ❌ Wrong — one giant test checking everything
   func TestService_CancelOrder(t *testing.T) {
       // 200 lines testing every scenario in sequence ...
   }
   ```

6. **Duplication is cheaper than the wrong abstraction.** Repeat setup in each subtest rather than building a shared helper that obscures intent. A `newTestService(t)` helper that returns all mocks is fine; a helper that also sets up mock expectations is not.

7. **Compare entire objects, not individual fields:**

   ```go
   // ✅ Correct — compare the whole struct
   expected := []order.Item{
       {ID: itemID, OrderID: orderID, ProductName: "Widget", Price: 5000, Quantity: 2, Subtotal: 10000},
   }
   assert.Equal(t, expected, result)

   // ❌ Wrong — assert field by field
   assert.Equal(t, "Widget", result[0].ProductName)
   assert.Equal(t, int64(5000), result[0].Price)
   assert.Equal(t, 2, result[0].Quantity)
   ```

### Test Speed Rules

Tests must stay fast. Follow these rules to avoid slow tests:

8. **Use `bcrypt.MinCost` in tests.** `bcrypt.DefaultCost` (10) costs ~250ms per hash. Pre-hash sample passwords with `bcrypt.MinCost` (4) and inject the hash via the mock `UserProvider`. Group tests that exercise the real `Register` path (which uses `DefaultCost`) into a single subtest to limit total runtime.

   ```go
   hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
   users.EXPECT().GetByEmail(mock.Anything, "user@example.com").
       Return(auth.UserCredentials{ID: id, PasswordHash: string(hash), Active: true}, nil)
   ```

9. **Use `testing/synctest` for time-dependent tests.** When testing code that uses `time.NewTicker`, `time.Sleep`, or `time.After`, wrap the subtest body in `synctest.Test` so the fake clock advances instantly instead of waiting for real wall-clock time.

   ```go
   // ✅ Correct — completes instantly
   t.Run("processes jobs", func(t *testing.T) {
       synctest.Test(t, func(t *testing.T) {
           ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
           defer cancel()
           w.Start(ctx) // ticker-based loop exits instantly via fake clock
       })
   })

   // ❌ Wrong — waits 5 real seconds
   t.Run("processes jobs", func(t *testing.T) {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       w.Start(ctx)
   })
   ```

10. **Set short timeouts on intentionally-broken connections.** When creating clients that connect to unreachable addresses (e.g., `localhost:1`) to test error paths, always set `MaxRetries: 0` and a short `DialTimeout`. Never rely on default timeouts for expected-failure tests.

    ```go
    // ✅ Correct — fails fast (~200ms)
    brokenRedis := redis.NewClient(&redis.Options{
        Addr:        "localhost:1",
        MaxRetries:  0,
        DialTimeout: 200 * time.Millisecond,
    })

    // ❌ Wrong — retries 3× with 5s dial timeout (~20s)
    brokenRedis := redis.NewClient(&redis.Options{Addr: "localhost:1"})
    ```

## Security & Compliance

- Secrets: Loaded from environment variables or `.env` file (gitignored). Never commit real secrets.
- Authentication: JWT tokens with configurable expiration. Passwords hashed with bcrypt.
- Authorization: Role-Based Access Control (RBAC) — admin middleware for admin-only endpoints.
- Middleware: Panic recovery, request-ID injection, structured request logging, CORS, rate limiting.

## Agent Guardrails

- Never modify files in `mocks/` by hand — always regenerate with `make mocks`.
- Never commit `.env`, secrets, or API keys.
- Always run `make test` (or at minimum `make vet`) before considering a change complete.
- Do not add third-party routers; the project uses `net/http` ServeMux intentionally.
- Do not suppress lint or vet errors with `//nolint` without a justification comment.
- Preserve the vertical-slice structure: each feature module is self-contained with handler → service → repository interface. PostgreSQL repository implementations are embedded in each feature package.
- Cross-feature dependencies must use inline interfaces at the top of the consumer's `service.go`. Never import another feature's concrete types directly.
- When adding a new feature, create a package under `internal/features/`, register routes in `internal/server/router.go`, and add adapters for cross-feature deps.

## Extensibility Hooks

- Environment variables: All config is driven by env vars with sensible defaults (see `.env.example` and `internal/config/config.go`).
- New feature modules: Add a package under `internal/features/`, define the repository interface there, implement the PostgreSQL repository in the same package, and register routes in `internal/server/router.go`.
- Worker tuning: `WORKER_INTERVAL`, `WORKER_BATCH_SIZE`, `WORKER_LEASE_DURATION`, `WORKER_CONCURRENCY` control the payment worker without code changes.
- Middleware chain: Add new middleware in `internal/middleware/` and wire it in `internal/server/router.go`.
- Payment gateways: Implement the `payment.Gateway` interface in `internal/platform/payment/` and swap via `PAYMENT_GATEWAY` env var.

## Further Reading

- [README.md](README.md) — API endpoint reference and quick-start instructions.
- [db/migrations/](db/migrations/) — Numbered SQL migration files.
- [.env.example](.env.example) — All supported environment variables with defaults.
- [.mockery.yml](.mockery.yml) — Mock generation configuration.
