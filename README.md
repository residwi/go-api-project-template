# Go API Project Template (ecommerce)

A production-ready Go API template built on feature modules with machine-checked layers.

## Features

- **Go 1.26+** with the new `ServeMux` routing
- **Feature modules with machine-checked layers** — 14 features, one `Service` each, plus a `checkout` bounded context and a `money` shared kernel
- **Two binaries**: API server (`cmd/api`) and Payment Job Worker (`cmd/worker`)
- **PostgreSQL 16+** with `pgx/v5` driver (requires `gen_random_uuid()`)
- **Redis 8.0+** caching with `go-redis/v9` (requires `HSETEX`)
- **JWT Authentication** with RBAC (Role-Based Access Control)
- **Database Migrations** with `goose`
- **Structured Logging** with `log/slog`
- **Request Validation** with `go-playground/validator`
- **Centralized Configuration** with `godotenv` + `envconfig`
- **Standard JSON Response Envelope**
- **Pagination Support**
- **Payment Gateway** abstraction with webhook support
- **Docker & Docker Compose** setup
- **Hot Reload** with Air
- **Generated Mocks** with mockery v3

## Project Structure

Feature modules sit under `/internal/features` — one directory per module,
sixteen of them. A module is **one flat package plus an `adapter/`
directory**: `service.go` declaring one exported `Service`, `repository.go`
for its storage port, `ports.go` for what it needs from other modules,
`contract.go` for what other modules may name, a `domain/` for its aggregate,
and an `adapter/` holding one subpackage per technology it speaks. Adapters
are named for their technology and a module only has the ones it needs, so the
tree is deliberately **non-uniform** — `auth` has no store at all, `user` has
two.

**A module names no URL.** Every route in the system is declared in
`/internal/server/router.go` — one function, 65 routes, fifteen labelled
blocks. A module supplies a handler with exported route methods; the transport
decides the verb, the path and the middleware group.

```text
/go-api-project-template
├── /cmd
│   ├── /api                    # API server entry point
│   ├── /worker                 # Payment + notification + order job worker
│   └── /mockgateway            # Dev-only mock payment gateway
├── /internal
│   ├── /modules                # 16 directories: 14 features, plus:
│   │   ├── /checkout           #   a bounded context orchestrating order+payment
│   │   ├── /money              #   a shared kernel: the Money value object
│   │   └── /auth /user /category /product /inventory /cart /order /payment
│   │       /review /promotion /wishlist /notification /dashboard /shipping
│   │       ├── service.go           # one exported Service and New; New takes
│   │       │                        # positional parameters, no Deps struct
│   │       ├── repository.go        # the storage port; adapter/postgres satisfies it
│   │       ├── ports.go             # what this module needs from others (9 of 16)
│   │       ├── contract.go          # what other modules may name (8 of 16)
│   │       ├── config.go            # this module's own env vars (4 of 16)
│   │       ├── domain/              # aggregate types + rules; the innermost
│   │       │                        # ring, touches no infrastructure (14 of 16)
│   │       ├── queue.go             # the outbound job port -- payment, notification only
│   │       └── adapter/
│   │           ├── postgres/        # SQL adapter, where the module has SQL (13)
│   │           ├── http/            # handlers + their wire types (15)
│   │           ├── redis/           # user only: its StatusCache port's store
│   │           ├── gateway/         # payment only: the outbound Gateway port
│   │           ├── channel/         # notification only: the outbound Channel port
│   │           └── jobs/            # job args + river.Worker -- payment, notification, order (3)
│   ├── /apperror               # Seven cross-module business sentinels, each a
│   │                           # wrap of a platform/errs kind
│   ├── /app                    # The composition root: builds every Service,
│   │                           # wires every cross-module port by name-match
│   ├── /config                 # This app's infra env vars -- rewritten per project,
│   │                           # which is why it sits outside /platform
│   ├── /money                  # The Money value object: a shared kernel every
│   │                           # module may name and that names none of them
│   ├── /server                 # server.go (Run) and router.go (NewRouter, health,
│   │                           # routes). It mounts middleware; it holds none
│   ├── /worker                 # The river.Client analogue of internal/server: owns
│   │                           # the one working client, the queue map, and the
│   │                           # order stale-sweep's river.PeriodicJob
│   ├── /platform               # Infrastructure, no domain knowledge. Module-owned
│   │   │                       # config (JWT, cart limits, payment gateway, ...)
│   │   │                       # lives in each module's own config.go
│   │   ├── /database           # Postgres pools, transactions, TxRunner
│   │   ├── /errs               # The five status-carrying generic error kinds
│   │   ├── /identity           # Identity: the UserID and Role a caller carries
│   │   ├── /queue              # NewInsertClient + a transaction-aware Insert
│   │   ├── /web                # Middleware, Chain, Router -- a tree of its own:
│   │   │   ├── /request        #   Bind (validator included), ParseUUIDParam
│   │   │   ├── /response       #   the envelope, HandleErr, CursorPage
│   │   │   └── /middleware     #   CORS, Logging, Recovery, RequestID, Auth and
│   │   │                       #   the identity context, Require/RequireRole,
│   │   │                       #   RateLimit
│   │   └── /cache /logger /paging /slug /storage
│   └── /testutil               # Shared container plumbing for tests
├── /test/e2e                   # Cross-module sagas through the real router
├── /db
│   ├── /migrations             # goose migrations
│   └── /seeds                  # Seed data
├── .go-arch-lint.yml           # Layer rules, enforced by `make check-arch`
├── AGENTS.md                   # Working rules, and which are machine-checked
└── ARCHITECTURE.md             # Why the codebase is shaped this way, what it
                                #   costs, and what it makes hard
```

No module holds a directory at its root outside `domain/` and `adapter/` —
`payment/gateway/` (the outbound `Gateway` port plus its three real
implementations — `stripe/ midtrans/ mock/`, picked once from
`PAYMENT_GATEWAY`) lives under `payment/adapter/gateway/` now. Background
jobs run on [River](https://riverqueue.com) rather than a hand-rolled queue:
`internal/platform/queue` holds exactly two functions — an insert-only
`river.Client` constructor and a transaction-aware `Insert` — and each of
`payment`, `notification` and `order` gained an `adapter/jobs` holding its
job args and its `river.Worker`. Payment and notification each declare their
outbound port in their own root `queue.go`; order enqueues nothing, since its
stale sweep is a `river.PeriodicJob` instead. `internal/worker` is new: it
owns the one working `river.Client` for the process, the queue map, and that
periodic job.

Mocks are generated by mockery v3 as a private `mocks_test.go` beside the
interface they mock, in-package — there is no top-level `/mocks` directory.

`make check-arch` runs [go-arch-lint](https://github.com/fe3dback/go-arch-lint)
against `.go-arch-lint.yml` and fails the build on any import that crosses a
ring the wrong way. The rules are about layers, not modules:

- `domain/` is the innermost ring. It depends on nothing — not infrastructure,
  and not another feature. A domain package importing anything of ours fails.
- A `Service` depends on the ports it declares, never on the adapters that
  implement them. A service importing its own `adapter/postgres` fails.
- Transport and drivers — `platform/web`, `queue`, `cache`, `storage` — are
  reachable from adapters and the wiring layer (`internal/app`,
  `internal/server`, `internal/worker`, `internal/testutil`), never from a
  `Service`. A service importing any of them fails.
- No feature may import `internal/server`, so no binary links HTTP just by
  constructing a module.

A component whose glob matches no directory is a hard config error, so the
config cannot drift from the tree. It reads imports and nothing else: wire-tag
placement, table ownership and module privacy are conventions `AGENTS.md`
records and no tool enforces.

## Getting Started

### Prerequisites

- Go 1.26 or later
- PostgreSQL 16+
- Redis 8.0+
- Docker & Docker Compose
- Make (optional but recommended)

### Quick Start

1. **Clone the repository**

   ```bash
   git clone https://github.com/residwi/go-api-project-template.git
   cd go-api-project-template
   ```

2. **Copy environment file**

   ```bash
   cp .env.example .env
   ```

3. **Option A: Run locally** (postgres & redis in Docker, app on host)

   ```bash
   make docker-up       # Start postgres and redis
   make migrate-up      # Run migrations
   make dev             # Run API with hot reload (Air)
   make run-worker      # Run worker in another terminal
   ```

4. **Option B: Run everything in Docker** (with hot reload via Air)

   ```bash
   make docker-dev      # Start all services (postgres, redis, api, worker)
   ```

## API Endpoints

#### Health Check

```
GET /health
```

#### Authentication

```
POST /api/auth/register         # Register new user
POST /api/auth/login            # Login
POST /api/auth/refresh          # Refresh token
```

#### Users (Authenticated)

```
GET /api/users/me               # Get own profile
PUT /api/users/me               # Update own profile
```

#### Users (Admin)

```
GET    /api/admin/users          # List users
GET    /api/admin/users/{id}     # Get user
PUT    /api/admin/users/{id}     # Update user
PUT    /api/admin/users/{id}/role # Update user role
DELETE /api/admin/users/{id}     # Delete user
```

#### Categories (Public)

```
GET /api/categories              # List categories
GET /api/categories/{slug}       # Get category by slug
```

#### Categories (Admin)

```
POST   /api/admin/categories           # Create category
PUT    /api/admin/categories/{id}      # Update category
DELETE /api/admin/categories/{id}      # Delete category
```

#### Products (Public)

```
GET /api/products                # List products
GET /api/products/{slug}         # Get product by slug
```

#### Products (Admin)

```
POST   /api/admin/products             # Create product
GET    /api/admin/products             # List products (admin)
GET    /api/admin/products/{id}        # Get product
PUT    /api/admin/products/{id}        # Update product
DELETE /api/admin/products/{id}        # Delete product
```

#### Inventory (Admin)

```
GET /api/admin/inventory/{product_id}           # Get stock
PUT /api/admin/inventory/{product_id}/restock   # Restock
PUT /api/admin/inventory/{product_id}/adjust    # Adjust stock
```

#### Cart (Authenticated)

```
GET    /api/cart                        # Get cart
POST   /api/cart/items                  # Add item
PUT    /api/cart/items/{product_id}     # Update item quantity
DELETE /api/cart/items/{product_id}     # Remove item
DELETE /api/cart                        # Clear cart
```

#### Orders (Authenticated)

```
POST /api/orders                       # Place order
GET  /api/orders                       # List my orders
GET  /api/orders/{id}                  # Get order detail
POST /api/orders/{id}/pay              # Retry payment
POST /api/orders/{id}/cancel           # Cancel order
```

#### Orders (Admin)

```
GET /api/admin/orders                  # List all orders
GET /api/admin/orders/{id}             # Get order detail
PUT /api/admin/orders/{id}/status      # Update order status
```

#### Payments (Public)

```
POST /api/payments/webhook             # Payment webhook callback
```

#### Payments (Admin)

```
GET  /api/admin/payments               # List payments
GET  /api/admin/payments/{id}          # Get payment detail
POST /api/admin/payments/{id}/refund   # Refund payment
```

#### Shipping (Authenticated)

```
GET /api/orders/{id}/shipping          # Get shipping info
```

#### Shipping (Admin)

```
POST /api/admin/orders/{id}/ship       # Create shipment
PUT  /api/admin/shipments/{id}/tracking # Update tracking
POST /api/admin/shipments/{id}/deliver  # Mark delivered
```

#### Reviews (Public + Authenticated)

```
GET  /api/products/{id}/reviews        # List product reviews
POST /api/products/{id}/reviews        # Create review (auth required)
```

#### Reviews (Admin)

```
DELETE /api/admin/reviews/{id}         # Delete review
```

#### Promotions (Authenticated)

```
POST /api/promotions/apply             # Apply coupon
```

#### Promotions (Admin)

```
POST   /api/admin/promotions           # Create promotion
GET    /api/admin/promotions           # List promotions
PUT    /api/admin/promotions/{id}      # Update promotion
DELETE /api/admin/promotions/{id}      # Delete promotion
```

#### Wishlist (Authenticated)

```
GET    /api/wishlist                   # Get wishlist
POST   /api/wishlist/items             # Add to wishlist
DELETE /api/wishlist/items/{product_id} # Remove from wishlist
```

#### Notifications (Authenticated)

```
GET /api/notifications                 # List notifications
PUT /api/notifications/{id}/read       # Mark as read
PUT /api/notifications/read-all        # Mark all as read
GET /api/notifications/unread-count    # Get unread count
```

#### Dashboard (Admin)

```
GET /api/admin/dashboard/summary       # Dashboard summary
GET /api/admin/dashboard/top-products  # Top selling products
GET /api/admin/dashboard/revenue       # Revenue analytics
```

### API Response Format

All responses follow a standard envelope format:

**Success Response:**

```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 100,
      "total_pages": 5
    }
  },
  "timestamp": "2024-01-01T00:00:00Z",
  "request_id": "abc123"
}
```

**Error Response:**

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": [
      {
        "field": "email",
        "message": "email is required"
      }
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z",
  "request_id": "abc123"
}
```

### Query Parameters

#### Pagination

```
GET /api/products?page=1&page_size=20
```

#### Sorting

```
GET /api/products?sort_by=created_at&sort_dir=desc
```

#### Filtering

```
GET /api/products?category=electronics&q=phone
```

### Authentication

Include the JWT token in the Authorization header:

```
Authorization: Bearer <access_token>
```

## Worker (payment + notification + order job queues)

The project includes a separate worker binary built on
[River](https://riverqueue.com), a Postgres-backed job queue. `cmd/worker`
is a thin `main` calling `worker.Run()`; `internal/worker` builds the one
working `river.Client` for the process, registers one `river.Worker` per job
kind — `payment/adapter/jobs.RefundWorker`, `notification/adapter/jobs.SendWorker`,
`order/adapter/jobs.ExpireStaleWorker` — and runs three queues (`payment`,
`notification`, `order`) side by side.

```bash
# Build worker
make build-worker

# Run worker
make run-worker
```

Payment and notification enqueue through their own outbound port
(`payment.Queue`, `notification.Queue`), each implemented in the
module's own `adapter/jobs` package against
`internal/platform/queue.Insert` — which joins the caller's open transaction
when there is one (type-asserting `database.PrimaryDB` to `pgx.Tx`), so an
enqueue never survives a business write that rolls back. Order enqueues
nothing: its stale sweep is a `river.PeriodicJob` declared in
`internal/worker`, run on `ORDER_JOB_INTERVAL` with `RunOnStart: true`, so a
long outage yields one sweep on restart rather than a backlog of missed
runs — the schedule is no longer a durable row the way the old
self-scheduling job's dedup key was.

River owns fetch batching, retries and its own maintenance, so there is no
`WORKER_BATCH_SIZE` or prune settings to configure any more. What is left:

- `WORKER_RESCUE_AFTER` — how long a job may sit `running` before River's
  client-wide rescuer reclaims it as stuck (default: 5m). The worker refuses
  to start if this is not below the order module's 15-minute
  stale-processing threshold, since a rescue that fires while a charge is
  genuinely still in flight can revert an order out from under it.
- `PAYMENT_JOB_CONCURRENCY` / `NOTIFICATION_JOB_CONCURRENCY` /
  `ORDER_JOB_CONCURRENCY` — max concurrent workers per queue.
- `PAYMENT_JOB_TIMEOUT` / `NOTIFICATION_JOB_TIMEOUT` / `ORDER_JOB_TIMEOUT` —
  each worker's own `Timeout()`, validated in that module's `LoadConfig`
  (payment's must be at least 3× `PAYMENT_GATEWAY_TIMEOUT`).
- `ORDER_JOB_INTERVAL` — the stale sweep's recurrence period.
- `PAYMENT_JOB_INTERVAL` / `NOTIFICATION_JOB_INTERVAL` — still validated
  (must be at least 5s) but no longer read anywhere: River's own client
  polls continuously and does not take a per-queue interval, and nothing
  else in `internal/worker` consumes these two fields today.

## Available Make Commands

```bash
make help             # Show all commands
make build            # Build all binaries (API + worker)
make build-api        # Build the API server
make build-worker     # Build the worker
make run              # Run the API server
make run-worker       # Run the worker
make dev              # Run with hot reload
make test             # Run tests (requires Docker)
make test-one NAME=X  # Run tests matching NAME, with .env loaded
make test-coverage    # Run tests with coverage
make test-clean       # Remove the shared postgres + redis test containers
make check-arch       # Run the architectural layer rules
make lint             # Run linter
make fmt              # Format code
make vet              # Run go vet
make tidy             # Tidy go modules
make mocks            # Generate mocks
make docker-up        # Start postgres and redis
make docker-dev       # Start all services with hot reload (API + worker)
make docker-down      # Stop all services
make docker-build     # Build Docker image
make docker-logs      # View logs
make docker-clean     # Clean up Docker resources
make migrate-up       # Run migrations
make migrate-jobs     # Run River's own migrations (river_job, river_migration, ...)
make migrate-down     # Rollback last migration
make migrate-down-all # Rollback all migrations
make migrate-create name=xxx  # Create new migration
make migrate-status   # Show migration status
make migrate-version  # Show current migration version
make db-create        # Create the database
make db-drop          # Drop the database
make setup            # Setup development environment
make deps             # Download dependencies
make all              # Run all checks and build
make ci               # Run CI pipeline
```

## Database Migrations

Create a new migration:

```bash
make migrate-create name=add_new_table
```

Run migrations:

```bash
make migrate-up
```

River manages its own job-queue tables (`river_job`, `river_migration`, ...)
separately from goose. Run its migrations once the database exists:

```bash
make migrate-jobs
```

Rollback one migration:

```bash
make migrate-down
```

> **Note:** Requires PostgreSQL 16+ for `gen_random_uuid()` support.

## Testing

**Docker is required.** There are no build tags and no short mode:
`internal/testutil` starts two long-lived containers by fixed name
(`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to
whichever already exists. Remove them with `make test-clean`.

```bash
make test              # everything, with -race
make test-one NAME=X   # just the tests matching X, with .env loaded
make test-coverage     # ./internal/... ./test/... -> coverage.out + coverage.html
```

`make all` is the gate before calling a change complete: it runs `fmt`, `vet`,
`check-arch`, `lint`, `test` and `build`. `make ci` runs the same checks
without `build`.

## Environment Variables

See `.env.example` for all available configuration options.

Key variables:

| Variable                        | Description                                    | Default                                                   |
| ------------------------------- | ---------------------------------------------- | --------------------------------------------------------- |
| `APP_NAME`                      | Application name                               | `ecommerce-api`                                           |
| `APP_ENV`                       | Environment (development, staging, production) | `development`                                             |
| `APP_PORT`                      | Server port                                    | `8080`                                                    |
| `APP_READ_TIMEOUT`              | HTTP read timeout                              | `15s`                                                     |
| `APP_WRITE_TIMEOUT`             | HTTP write timeout                             | `15s`                                                     |
| `APP_IDLE_TIMEOUT`              | HTTP idle timeout                              | `60s`                                                     |
| `APP_SHUTDOWN_TIMEOUT`          | Graceful shutdown timeout                      | `30s`                                                     |
| `MAX_CART_ITEMS`                | Maximum items per cart                         | `50`                                                      |
| `ORDER_RATE_LIMIT`              | Order rate limit per user                      | `5`                                                       |
| `ORDER_RATE_WINDOW`             | Order rate limit window                        | `1m`                                                      |
| `DB_HOST`                       | Database host                                  | `localhost`                                               |
| `DB_PORT`                       | Database port                                  | `5432`                                                    |
| `DB_USER`                       | Database user                                  | `postgres`                                                |
| `DB_PASSWORD`                   | Database password                              | `postgres`                                                |
| `DB_NAME`                       | Database name                                  | `ecommerce`                                               |
| `DB_SSLMODE`                    | Database SSL mode                              | `disable`                                                 |
| `DB_MAX_CONNS`                  | Max database connections                       | `25`                                                      |
| `DB_MIN_CONNS`                  | Min database connections                       | `5`                                                       |
| `DB_MAX_CONN_LIFETIME`          | Max connection lifetime                        | `1h`                                                      |
| `DB_MAX_CONN_IDLE_TIME`         | Max connection idle time                       | `30m`                                                     |
| `DB_STATEMENT_TIMEOUT`          | Statement timeout                              | `30s`                                                     |
| `DB_IDLE_IN_TX_SESSION_TIMEOUT` | Idle in transaction timeout                    | `60s`                                                     |
| `REPLICA_DATABASE_URL`           | Read replica URL — now load-bearing: set it and `dashboard`, plus the read-only paths of `order`, `product`, `promotion` and `user`, read from the replica instead of the primary | — |
| `REDIS_HOST`                    | Redis host                                     | `localhost`                                               |
| `REDIS_PORT`                    | Redis port                                     | `6379`                                                    |
| `REDIS_PASSWORD`                | Redis password                                 | —                                                         |
| `REDIS_DB`                      | Redis database index                           | `0`                                                       |
| `JWT_SECRET`                    | JWT signing key                                | —                                                         |
| `JWT_ACCESS_TTL`                | Access token TTL                               | `15m`                                                     |
| `JWT_REFRESH_TTL`               | Refresh token TTL                              | `168h`                                                    |
| `JWT_ISSUER`                    | JWT issuer                                     | `ecommerce-api`                                           |
| `AUTH_RATE_LIMIT`               | Login rate limit per IP                        | `10`                                                      |
| `AUTH_RATE_WINDOW`              | Login rate limit window                        | `1m`                                                      |
| `BCRYPT_COST`                   | Password hashing cost                          | `10`                                                      |
| `LOG_LEVEL`                     | Logging level (debug, info, warn, error)       | `info`                                                    |
| `LOG_FORMAT`                    | Log format (json, text)                        | `json`                                                    |
| `CORS_ALLOWED_ORIGINS`          | CORS allowed origins                           | `*`                                                       |
| `CORS_ALLOWED_METHODS`          | CORS allowed methods                           | `GET,POST,PUT,DELETE,OPTIONS`                             |
| `CORS_ALLOWED_HEADERS`          | CORS allowed headers                           | `Content-Type,Authorization,X-Request-ID,Idempotency-Key` |
| `CORS_MAX_AGE`                  | CORS max age (seconds)                         | `86400`                                                   |
| `WORKER_RESCUE_AFTER`           | How long a job can sit `running` before River's client-wide rescuer reclaims it as stuck | `5m` |
| `PAYMENT_JOB_INTERVAL`       | Validated (>= 5s), but unread since the move to River — see Worker section | `10s` |
| `PAYMENT_JOB_CONCURRENCY`    | Payment queue concurrent processors            | `5`                                                       |
| `PAYMENT_JOB_TIMEOUT`        | Payment refund worker's per-job timeout          | `2m`                                                      |
| `NOTIFICATION_JOB_INTERVAL`  | Validated (>= 5s), but unread since the move to River — see Worker section | `5s` |
| `NOTIFICATION_JOB_CONCURRENCY` | Notification queue concurrent processors     | `10`                                                      |
| `NOTIFICATION_JOB_TIMEOUT`   | Notification send worker's per-job timeout       | `30s`                                                     |
| `ORDER_JOB_INTERVAL`         | Order queue poll interval                      | `1m`                                                      |
| `ORDER_JOB_CONCURRENCY`      | Order queue concurrent processors              | `1`                                                       |
| `ORDER_JOB_TIMEOUT`          | Order stale-sweep worker's per-job timeout       | `2m`                                                      |
| `PAYMENT_GATEWAY`               | Payment gateway provider                       | `mock`                                                    |
| `PAYMENT_GATEWAY_URL`           | Payment gateway URL                            | —                                                         |
| `PAYMENT_GATEWAY_TIMEOUT`       | Payment gateway timeout                        | `10s`                                                     |
| `PAYMENT_GATEWAY_API_KEY`       | Payment gateway API key                        | —                                                         |
| `PAYMENT_WEBHOOK_SECRET`        | Payment webhook secret                         | —                                                         |

## Architecture

This template puts one module per feature and one `Service` per module:

- Each feature (auth, user, product, order, …) is a module with one exported
  `Service`, its own repository port, its own `domain/`, and its own wire DTOs
  in `adapter/http`. `checkout` is the exception that proves the rule: it owns
  no table and no domain, and exists to orchestrate `order` and `payment`
  across one business transaction — which is what keeps those two from
  importing each other.
- Dependencies flow inward (handler → `Service` → repository), and URLs flow
  the other way: `internal/server` imports the modules, never the reverse, and
  a boundary check enforces the direction.
- PostgreSQL adapters live in each module's `adapter/postgres`, so a `Service`
  *cannot* import its own SQL adapter without a compile-time import cycle.
  `internal/app` builds the adapter and hands it to `Service`, so the
  pool never reaches the module's root package.
- Cross-module dependencies use interfaces declared by the **consumer**, in
  its own `ports.go` — `internal/features/order/ports.go` declares what `order`
  needs from `cart`; `cart` publishes none of it. Nine modules have a
  `ports.go`; the other seven reach nothing outside themselves. Two mechanisms
  satisfy a port, and `internal/app` (the composition root, shared by
  the API server and the worker) wires them once: **name-match**, when the
  producer's own value already has a method named what the port asks for
  (`promotion.Service` satisfies both `order.CouponReserver` and
  `payment.CouponReleaser` with no adapter), or a **type in the producer's
  `contract.go`**, when what crosses is a struct rather than something a value
  already satisfies. The dependency graph stays acyclic by construction.
- Order status changes from other modules go through named `domain.Transition`
  values applied via `order.Service.Apply` — payment and shipping express
  intent (`MarkPaid`, `MarkRefunded`, `MarkShipped`, …) against their own
  port, and `order.Service` turns each intent into the right transition
  internally, keeping the state machine's allowed transitions in one place
  (`internal/features/order/domain/state.go`).
- Monetary amounts are `money.Money` (an amount paired with its currency) in
  `order`, `payment`, `product` and `cart`, so an amount cannot drift from the
  currency beside it. `promotion` and `dashboard` stay on `int64` for reasons
  recorded in `ARCHITECTURE.md` §10.
- Configuration is validated at startup and boot aborts on failure:
  infra-level settings in `Infra.validate()` (`internal/config`),
  module-owned settings (a sub-second `AUTH_RATE_WINDOW`, a
  `PAYMENT_JOB_CONCURRENCY < 1`) inline in that module's own `LoadConfig`
  (`auth.LoadConfig`, `cart.LoadConfig`, `notification.LoadConfig`,
  `order.LoadConfig`, `payment.LoadConfig`).
- The five generic error kinds live in `internal/platform/errs` and the seven
  cross-module business sentinels in `internal/apperror`, each declared as a
  wrap of one of the five; generic utilities (`paging`, `slug`) live in
  `internal/platform`; the JSON envelope lives in
  `internal/platform/web/response`, and request binding plus struct
  validation in `internal/platform/web/request` -- `Bind` holds the one
  `go-playground/validator` instance, so no handler carries one.

**Read `ARCHITECTURE.md`'s "Limitations" section before copying this.** It
lists what the
shape makes hard, including the guarantees the flat-module shape gave up — a
module's whole exported surface is reachable from every other module, and
nothing but review keeps a cross-module call inside a declared port.

## Security

- Passwords are hashed using bcrypt
- JWT tokens with configurable expiration
- Role-Based Access Control (RBAC)
- Request ID tracking
- Panic recovery middleware
- Request size limiting
- Idempotency key support
- Payment webhook signature verification

## License

MIT License
