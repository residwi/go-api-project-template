# Go API Project Template (ecommerce)

A production-ready Go API template built on feature modules with machine-checked boundaries.

## Features

- **Go 1.26+** with the new `ServeMux` routing
- **Feature modules with machine-checked boundaries** — 14 features, one `Service` each, plus a `checkout` bounded context and a `money` shared kernel
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

Feature modules sit under `/internal/modules` — one directory per module,
sixteen of them. A module is **one flat package plus an `adapter/`
directory**: `service.go` declaring one exported `Service`, `repository.go`
for its storage port, `ports.go` for what it needs from other modules,
`contract.go` for what other modules may name, a `domain/` for its aggregate,
and an `adapter/` holding one subpackage per technology it speaks. Adapters
are named for their technology and a module only has the ones it needs, so the
tree is deliberately **non-uniform** — `auth` has no store at all, `user` has
two.

**A module names no URL.** Every route in the system is declared in
`/internal/server/routes.go` — one function, 64 routes, fifteen labelled
blocks. A module supplies a handler with exported route methods; the transport
decides the verb, the path and the middleware group.

```text
/go-api-project-template
├── /cmd
│   ├── /api                    # API server entry point
│   ├── /worker                 # Payment + notification job worker
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
│   │       ├── domain/              # aggregate types + rules; private, and check 4
│   │       │                        # enforces that (14 of 16)
│   │       ├── job.go               # a job's Kind + Run; payment, notification only
│   │       └── adapter/
│   │           ├── postgres/        # SQL adapter, where the module has SQL (13)
│   │           ├── http/            # handlers + their wire types (15)
│   │           ├── redis/           # user only: its StatusCache port's store
│   │           ├── gateway/         # payment only: the outbound Gateway port
│   │           └── channel/         # notification only: the outbound Channel port
│   ├── /apperror               # Error vocabulary (ErrNotFound, ErrBadRequest, ...)
│   ├── /bootstrap              # The composition root: builds every Service,
│   │                           # wires every cross-module port by name-match
│   ├── /server                 # server.go (Run, NewRouter, health), routes.go,
│   │   ├── /middleware         # recovery, request ID, logging, CORS, rate limit,
│   │   │                       # auth, admin
│   │   ├── /response           # the shared JSON envelope + Bind + error mapping
│   │   └── /testdata           # routes.golden -- 64 routes, method/path/group
│   ├── /platform               # Infrastructure, no domain knowledge
│   │   ├── /config             # Infra config (godotenv + envconfig) -- module-owned
│   │   │                       # config (JWT, cart limits, payment gateway, ...)
│   │   │                       # lives in each module's own config.go instead
│   │   ├── /database           # Postgres pools, transactions, TxRunner
│   │   └── /cache /jobs /logger /paging /slug /storage /validator
│   └── /testutil               # Shared container plumbing for tests
├── /test/e2e                   # Cross-module sagas through the real router
├── /db
│   ├── /migrations             # goose migrations
│   ├── /seeds                  # Seed data
│   └── OWNERSHIP.md            # Which module owns which table (machine-checked)
├── /scripts                    # check-boundaries.sh + its probe test
├── AGENTS.md                   # Working rules, and which are machine-checked
├── ARCHITECTURE.md             # Why the codebase is shaped this way
└── ARCHITECTURE-LIMITATIONS.md # What that shape costs you
```

No module holds a directory at its root outside `domain/` and `adapter/` —
`payment/gateway/` (the outbound `Gateway` port plus its three real
implementations — `stripe/ midtrans/ mock/`, picked once from
`PAYMENT_GATEWAY`) lives under `payment/adapter/gateway/` now, and the two
job-queue directories that used to sit beside it — `payment/jobs/postgres/`
and `notification/jobs/` — are gone. Background jobs share one
platform-owned queue instead: a `job_queue` table, an `internal/platform/jobs`
`Store`/`Registry`/`Runner`, and one `job.go` per module declaring its own
job type (`payment.RefundJob`, `notification.SendJob`).

Mocks are generated by mockery v3 as a private `mocks_test.go` beside the
interface they mock, in-package — there is no top-level `/mocks` directory.

`make check-boundaries` runs **five** checks and fails the build on any of
them. They are numbered 1, 2, 3, 4 and 6; the gaps are where two checks were
retired, and renumbering would falsify every by-number citation in `AGENTS.md`,
`ARCHITECTURE.md` and `db/OWNERSHIP.md` at once.

1. No `json` tag on a type this system owns outside a module's own
   `adapter/http`, no `json:"-"` anywhere under `internal/`, and no `dto.go`
   anywhere under `internal/`.
2. `db/OWNERSHIP.md` itself has no duplicate row, no row for a table no
   migration creates, and no table with no owning row.
3. No SQL anywhere in a module naming a table that module does not own.
4. A module may import another module's **root package** — that is its
   published surface — and nothing deeper: not its `domain/`, not any of its
   adapters. `internal/bootstrap` and `internal/server` are exempt as
   importers, and `checkout` alone may reach `order/domain`.
5. *(retired)* — it refused a slice importing a sibling slice; there are no
   slices.
6. A module may not import `internal/server`, except its own `adapter/http`.
7. *(retired)* — it kept each `contract/` package a leaf; published types live
   in `contract.go` in the module's root package now.

`scripts/boundaries_test.go` plants probe files in real modules and asserts
the script reports each one — and asserts that the legal cases stay clean — so
a path-keyed check that has quietly stopped matching anything fails a test
instead of printing `Boundaries OK`. See `AGENTS.md` for the full list, each
check's exact function name, and which rules are conventions rather than
checks.

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

## Worker (payment + notification job queues)

The project includes a separate worker binary that drains two job queues:

```bash
# Build worker
make build-worker

# Run worker
make run-worker
```

The worker runs two `platform/jobs.Runner` loops side by side, both draining
the shared `job_queue` table, one per queue name. One claims the `payment`
queue, dispatches each claimed job to a charge or a refund, and runs order's
stale-order housekeeping sweep once per tick. The other claims the
`notification` queue. Both use the same leased compare-and-set claim, bounded
concurrency, per-job timeout and pruning; neither hand-rolls a ticker.

Worker configuration via environment variables. `WORKER_BATCH_SIZE` and the
prune settings are shared; poll interval, lease and concurrency are per queue,
since payment and notification jobs run at different rates:

- `WORKER_BATCH_SIZE` — Jobs claimed per batch, both queues (default: 10)
- `WORKER_PAYMENT_INTERVAL` — Payment queue poll interval (default: 10s)
- `WORKER_PAYMENT_CONCURRENCY` — Payment queue concurrent processors (default: 5)
- `WORKER_PAYMENT_LEASE` — Payment queue job lease duration (default: 2m)
- `WORKER_NOTIFICATION_INTERVAL` — Notification queue poll interval (default: 5s)
- `WORKER_NOTIFICATION_CONCURRENCY` — Notification queue concurrent processors (default: 10)
- `WORKER_NOTIFICATION_LEASE` — Notification queue job lease duration (default: 30s)

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
make check-boundaries # Run the architectural boundary checks
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
`check-boundaries`, `lint`, `test` and `build`. **`make ci` does not run
`check-boundaries`** — if you rely on one command, use `make all`.

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
| `READER_DATABASE_URL`           | Read replica URL — now load-bearing: set it and `dashboard`, plus the read-only paths of `order`, `product`, `promotion` and `user`, read from the replica instead of the primary | — |
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
| `WORKER_BATCH_SIZE`             | Worker jobs per batch, both queues             | `10`                                                      |
| `WORKER_PAYMENT_INTERVAL`       | Payment queue poll interval                    | `10s`                                                     |
| `WORKER_PAYMENT_CONCURRENCY`    | Payment queue concurrent processors            | `5`                                                       |
| `WORKER_PAYMENT_LEASE`          | Payment queue job lease duration                | `2m`                                                      |
| `WORKER_NOTIFICATION_INTERVAL`  | Notification queue poll interval               | `5s`                                                      |
| `WORKER_NOTIFICATION_CONCURRENCY` | Notification queue concurrent processors     | `10`                                                      |
| `WORKER_NOTIFICATION_LEASE`     | Notification queue job lease duration          | `30s`                                                     |
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
  `internal/bootstrap` builds the adapter and hands it to `Service`, so the
  pool never reaches the module's root package.
- Cross-module dependencies use interfaces declared by the **consumer**, in
  its own `ports.go` — `internal/modules/order/ports.go` declares what `order`
  needs from `cart`; `cart` publishes none of it. Nine modules have a
  `ports.go`; the other seven reach nothing outside themselves. Two mechanisms
  satisfy a port, and `internal/bootstrap` (the composition root, shared by
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
  (`internal/modules/order/domain/transition.go`).
- Monetary amounts are `money.Money` (an amount paired with its currency) in
  `order`, `payment`, `product` and `cart`, so an amount cannot drift from the
  currency beside it. `promotion` and `dashboard` stay on `int64` for reasons
  recorded in `ARCHITECTURE.md` §10.
- Configuration is validated at startup and boot aborts on failure:
  infra-level settings in `Infra.validate()` (`internal/platform/config`),
  module-owned settings (a sub-second `AUTH_RATE_WINDOW`, a
  `WORKER_CONCURRENCY < 1`) inline in that module's own `LoadConfig`
  (`auth.LoadConfig`, `cart.LoadConfig`, `order.LoadConfig`,
  `payment.LoadConfig`).
- The error vocabulary lives in `internal/apperror`; generic utilities
  (`paging`, `slug`, `validator`) live in `internal/platform`; the JSON
  envelope and request binding live in `internal/server/response`.

**Read `ARCHITECTURE-LIMITATIONS.md` before copying this.** It lists what the
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
