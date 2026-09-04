# Go API Project Template (ecommerce)

A production-ready Go API template: a modular monolith of hexagonal feature modules, with layer rules enforced by go-arch-lint.

## Features

- **Go 1.26+** with the new `ServeMux` routing
- **Modular monolith** — feature modules, each a hexagon with its own domain, ports and adapters; layer rules enforced by go-arch-lint
- **Two binaries**: API server (`cmd/api`) and job worker (`cmd/worker`)
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

```text
/go-api-project-template
├── /cmd
│   ├── /api                    # API server entry point (server.Run)
│   ├── /worker                 # Job worker entry point (worker.Run)
│   └── /mockgateway            # Dev-only mock payment gateway
│       └── /mockserver         #   its handlers, mountable in-process
├── /internal
│   ├── /features               # One directory per feature module:
│   │   ├── /checkout           #   a bounded context orchestrating order+payment;
│   │   │                       #   owns no table, no domain/, no store
│   │   └── /auth /user /category /product /inventory /cart /order /payment
│   │       /shipping /review /promotion /wishlist /notification /dashboard
│   │       ├── service.go           # one exported Service and New; New takes
│   │       │                        #   positional parameters, no Deps struct
│   │       ├── repository.go        # the storage port; adapter/postgres satisfies it
│   │       ├── ports.go             # what this module needs from other modules
│   │       ├── contract.go          # the structs other modules may name
│   │       ├── config.go            # this module's own env vars
│   │       │                        #   (auth, cart, notification, order, payment)
│   │       ├── errors.go            # its own error sentinels (auth, payment)
│   │       ├── queue.go             # the outbound job port (notification, payment)
│   │       ├── channel.go           # the outbound Channel port (notification)
│   │       ├── gateway.go           # the outbound Gateway port (payment)
│   │       ├── cache.go             # the StatusCache port (user)
│   │       ├── domain/              # aggregate types + rules; the innermost ring,
│   │       │                        #   touches no infrastructure
│   │       └── adapter/             # only the subpackages the module needs:
│   │           ├── postgres/        #   SQL adapter
│   │           ├── http/            #   handlers + their wire types
│   │           ├── redis/           #   user only: the StatusCache store
│   │           ├── jwt/             #   auth only: the Tokens port
│   │           ├── gateway/         #   payment only: stripe/ midtrans/ mock/
│   │           ├── channel/         #   notification only: the log channel
│   │           └── jobs/           #   job args + river.Worker
│   │                               #   (notification, order, payment)
│   ├── /app                    # The composition root: builds every Service and
│   │                           # wires every cross-module port
│   ├── /apperror               # Cross-module business sentinels, each a wrap of
│   │                           # a platform/errs kind
│   ├── /config                 # This app's infra env vars -- rewritten per project,
│   │                           # which is why it sits outside /platform
│   ├── /money                  # The Money value object: every module may name it,
│   │                           # it names none of them
│   ├── /server                 # server.go (Run) and router.go (NewRouter, health,
│   │                           # every route). It mounts middleware; it holds none
│   ├── /worker                 # The river.Client analogue of /server: the one
│   │                           # working client, its queue map, its periodic job
│   ├── /platform               # Infrastructure, no feature knowledge:
│   │   ├── /database           #   pools, transactions, TxRunner, keyset helpers
│   │   ├── /cache              #   Redis client
│   │   ├── /jobqueue           #   NewInsertClient + a transaction-aware Insert
│   │   ├── /errs               #   the five status-carrying error kinds
│   │   ├── /identity           #   Identity (UserID, Role) and its context plumbing
│   │   ├── /logger             #   slog setup and context attributes
│   │   ├── /paging             #   cursor and offset pagination
│   │   ├── /slug /storage      #   slug generation, file storage
│   │   └── /web                #   Middleware, Chain, Router -- a tree of its own:
│   │       ├── /request        #     Bind (validator included), RequireUser,
│   │       │                   #     ParseUUIDParam
│   │       ├── /response       #     the envelope, HandleErr, CursorPage
│   │       └── /middleware     #     CORS, Logging, Recovery, RequestID, Auth,
│   │                           #     Require/RequireRole, RateLimit
│   └── /testutil               # Shared container plumbing for tests
├── /test/e2e                   # Cross-module sagas through the real router
├── /db
│   ├── /migrations             # goose migrations
│   └── /seeds                  # Seed data
├── .go-arch-lint.yml           # Layer rules, enforced by `make check-arch`
├── .mockery.yml                # In-package mock generation (make mocks)
├── AGENTS.md                   # Working rules for humans and agents
└── ARCHITECTURE.md             # Why the codebase is shaped this way
```

What a module holds, which imports `make check-arch` refuses, and what the
shape costs are all in **[ARCHITECTURE.md](ARCHITECTURE.md)**;
[AGENTS.md](AGENTS.md) carries the day-to-day working rules.

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
POST /api/checkout                     # Place an order and charge it
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

Every response is the same three-field envelope, declared once in
`internal/platform/web/response`:

**Success:**

```json
{
  "success": true,
  "data": { ... }
}
```

**Paginated list** — pagination sits inside `data`, beside `items`. Public
endpoints are cursor-paginated, admin endpoints offset-paginated:

```json
{
  "success": true,
  "data": {
    "items": [...],
    "pagination": { "next_cursor": "MjAyNi0wOS0wNFQx...", "has_more": true }
  }
}
```

```json
{
  "success": true,
  "data": {
    "items": [...],
    "pagination": { "current_page": 1, "page_size": 20, "total_items": 100, "total_pages": 5 }
  }
}
```

**Error:**

```json
{
  "success": false,
  "error": { "message": "order not found" }
}
```

**Validation error** (422) — `details` maps each field to one message:

```json
{
  "success": false,
  "error": {
    "message": "validation failed",
    "details": {
      "email": "must be a valid email address",
      "quantity": "this field is required"
    }
  }
}
```

### Query Parameters

**Pagination.** Cursor for public lists, offset for admin lists. Both default
to 20 per page and cap at 100:

```
GET /api/products?cursor=<opaque>&limit=20
GET /api/admin/products?page=1&page_size=20
```

**Filtering.** Each endpoint reads only the parameters it supports, and there is
no generic sort parameter:

```
GET /api/products?search=phone&category_id=<uuid>&min_price=1000&max_price=50000
GET /api/admin/orders?status=paid
GET /api/admin/payments?status=succeeded&order_id=<uuid>
GET /api/admin/users?search=ada&role=admin&active=true
GET /api/admin/dashboard/revenue?from=2026-01-01&to=2026-01-31
```

### Authentication

Include the JWT token in the Authorization header:

```
Authorization: Bearer <access_token>
```

## Worker (payment + notification + order job queues)

A separate binary on [River](https://riverqueue.com), a Postgres-backed queue.
`cmd/worker` is a thin `main` calling `worker.Run()`; `internal/worker` builds
the one `river.Client` for the process and runs the `payment`, `notification`
and `order` queues side by side.

```bash
# Build worker
make build-worker

# Run worker
make run-worker
```

Enqueueing goes through `platform/jobqueue.Insert`, which joins the caller's open
transaction when there is one, so an enqueue never survives a business write
that rolls back. Every `*_JOB_*` and `WORKER_*` setting is listed under
[Environment Variables](#environment-variables); one of them is a startup
invariant rather than a preference — the worker refuses to start unless
`WORKER_RESCUE_AFTER` is below the order module's stale-processing threshold,
since a rescue firing while a charge is still in flight can revert an order out
from under it. `ARCHITECTURE.md` decision 18 explains the rest.

## Available Make Commands

```bash
make help             # Show all commands
make build            # Build all binaries (API + worker)
make build-api        # Build the API server
make build-worker     # Build the worker
make build-mockgateway # Build the dev-only mock payment gateway
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
make vuln             # Run govulncheck
make tidy             # Tidy go modules
make clean            # Remove build output and coverage files
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
make seed             # Apply db/seeds/data.sql
make migrate-install  # Install the goose and river CLIs
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

`.env.example` is the exhaustive list. The table below covers everything except
the Redis connection-pool group (`REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`,
`REDIS_DIAL_TIMEOUT`, `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT`,
`REDIS_POOL_TIMEOUT`), which is tuning rather than configuration.

Key variables:

| Variable                        | Description                                                                                                                                                                       | Default                                                   |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `APP_NAME`                      | Application name                                                                                                                                                                  | `ecommerce-api`                                           |
| `APP_ENV`                       | Environment (development, staging, production)                                                                                                                                    | `development`                                             |
| `APP_PORT`                      | Server port                                                                                                                                                                       | `8080`                                                    |
| `APP_READ_TIMEOUT`              | HTTP read timeout                                                                                                                                                                 | `15s`                                                     |
| `APP_WRITE_TIMEOUT`             | HTTP write timeout                                                                                                                                                                | `15s`                                                     |
| `APP_IDLE_TIMEOUT`              | HTTP idle timeout                                                                                                                                                                 | `60s`                                                     |
| `APP_SHUTDOWN_TIMEOUT`          | Graceful shutdown timeout                                                                                                                                                         | `30s`                                                     |
| `MAX_CART_ITEMS`                | Maximum items per cart                                                                                                                                                            | `50`                                                      |
| `ORDER_RATE_LIMIT`              | Order rate limit per user                                                                                                                                                         | `5`                                                       |
| `ORDER_RATE_WINDOW`             | Order rate limit window                                                                                                                                                           | `1m`                                                      |
| `DB_HOST`                       | Database host                                                                                                                                                                     | `localhost`                                               |
| `DB_PORT`                       | Database port                                                                                                                                                                     | `5432`                                                    |
| `DB_USER`                       | Database user                                                                                                                                                                     | `postgres`                                                |
| `DB_PASSWORD`                   | Database password                                                                                                                                                                 | `postgres`                                                |
| `DB_NAME`                       | Database name                                                                                                                                                                     | `ecommerce`                                               |
| `DB_SSLMODE`                    | Database SSL mode                                                                                                                                                                 | `disable`                                                 |
| `DB_MAX_CONNS`                  | Max database connections                                                                                                                                                          | `25`                                                      |
| `DB_MIN_CONNS`                  | Min database connections                                                                                                                                                          | `5`                                                       |
| `DB_MAX_CONN_LIFETIME`          | Max connection lifetime                                                                                                                                                           | `1h`                                                      |
| `DB_MAX_CONN_IDLE_TIME`         | Max connection idle time                                                                                                                                                          | `30m`                                                     |
| `DB_STATEMENT_TIMEOUT`          | Statement timeout                                                                                                                                                                 | `30s`                                                     |
| `DB_IDLE_IN_TX_SESSION_TIMEOUT` | Idle in transaction timeout                                                                                                                                                       | `60s`                                                     |
| `REPLICA_DATABASE_URL`          | Read replica URL — now load-bearing: set it and `dashboard`, plus the read-only paths of `order`, `product`, `promotion` and `user`, read from the replica instead of the primary | —                                                         |
| `REDIS_HOST`                    | Redis host                                                                                                                                                                        | `localhost`                                               |
| `REDIS_PORT`                    | Redis port                                                                                                                                                                        | `6379`                                                    |
| `REDIS_PASSWORD`                | Redis password                                                                                                                                                                    | —                                                         |
| `REDIS_DB`                      | Redis database index                                                                                                                                                              | `0`                                                       |
| `JWT_SECRET`                    | JWT signing key                                                                                                                                                                   | —                                                         |
| `JWT_ACCESS_TTL`                | Access token TTL                                                                                                                                                                  | `15m`                                                     |
| `JWT_REFRESH_TTL`               | Refresh token TTL                                                                                                                                                                 | `168h`                                                    |
| `JWT_ISSUER`                    | JWT issuer                                                                                                                                                                        | `ecommerce-api`                                           |
| `AUTH_RATE_LIMIT`               | Login rate limit per IP                                                                                                                                                           | `10`                                                      |
| `AUTH_RATE_WINDOW`              | Login rate limit window                                                                                                                                                           | `1m`                                                      |
| `BCRYPT_COST`                   | Password hashing cost                                                                                                                                                             | `10`                                                      |
| `LOG_LEVEL`                     | Logging level (debug, info, warn, error)                                                                                                                                          | `info`                                                    |
| `LOG_FORMAT`                    | Log format (json, text)                                                                                                                                                           | `json`                                                    |
| `CORS_ALLOWED_ORIGINS`          | CORS allowed origins                                                                                                                                                              | `*`                                                       |
| `CORS_ALLOWED_METHODS`          | CORS allowed methods                                                                                                                                                              | `GET,POST,PUT,DELETE,OPTIONS`                             |
| `CORS_ALLOWED_HEADERS`          | CORS allowed headers                                                                                                                                                              | `Content-Type,Authorization,X-Request-ID,Idempotency-Key` |
| `CORS_MAX_AGE`                  | CORS max age (seconds)                                                                                                                                                            | `86400`                                                   |
| `WORKER_RESCUE_AFTER`           | How long a job can sit `running` before River's client-wide rescuer reclaims it as stuck                                                                                          | `5m`                                                      |
| `PAYMENT_JOB_INTERVAL`          | Validated (>= 5s), but unread since the move to River — see Worker section                                                                                                        | `10s`                                                     |
| `PAYMENT_JOB_CONCURRENCY`       | Payment queue concurrent processors                                                                                                                                               | `5`                                                       |
| `PAYMENT_JOB_TIMEOUT`           | Payment refund worker's per-job timeout                                                                                                                                           | `2m`                                                      |
| `NOTIFICATION_JOB_INTERVAL`     | Validated (>= 5s), but unread since the move to River — see Worker section                                                                                                        | `5s`                                                      |
| `NOTIFICATION_JOB_CONCURRENCY`  | Notification queue concurrent processors                                                                                                                                          | `10`                                                      |
| `NOTIFICATION_JOB_TIMEOUT`      | Notification send worker's per-job timeout                                                                                                                                        | `30s`                                                     |
| `ORDER_JOB_INTERVAL`            | Order queue poll interval                                                                                                                                                         | `1m`                                                      |
| `ORDER_JOB_CONCURRENCY`         | Order queue concurrent processors                                                                                                                                                 | `1`                                                       |
| `ORDER_JOB_TIMEOUT`             | Order stale-sweep worker's per-job timeout                                                                                                                                        | `2m`                                                      |
| `PAYMENT_GATEWAY`               | Payment gateway provider                                                                                                                                                          | `mock`                                                    |
| `PAYMENT_GATEWAY_URL`           | Payment gateway URL                                                                                                                                                               | —                                                         |
| `PAYMENT_GATEWAY_TIMEOUT`       | Payment gateway timeout                                                                                                                                                           | `10s`                                                     |
| `PAYMENT_GATEWAY_API_KEY`       | Payment gateway API key                                                                                                                                                           | —                                                         |
| `PAYMENT_WEBHOOK_SECRET`        | Payment webhook secret                                                                                                                                                            | —                                                         |

## Architecture

A modular monolith: one deployable, one database, and one flat package per
feature module, each module its own hexagon — a `Service` that declares the
ports it needs, with `adapter/` holding whatever satisfies them. Ports are
declared by the consumer, `internal/app` wires them, and `internal/server` owns
every URL so no module names one.

**[ARCHITECTURE.md](ARCHITECTURE.md)** has the decisions behind that shape,
what each one costs, and the limitations it creates — read its "Limitations"
section before copying this template.

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
