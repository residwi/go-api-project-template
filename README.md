# Go API Project Template (ecommerce)

A production-ready Go API template with Feature-Based Clean Architecture (Vertical Slicing).

## Features

- **Go 1.26+** with the new `ServeMux` routing
- **Feature-Based Clean Architecture** (Vertical Slicing) — 14 feature modules
- **Two binaries**: API server (`cmd/api`) and Payment Job Worker (`cmd/worker`)
- **PostgreSQL 16+** with `pgx/v5` driver (requires `gen_random_uuid()`)
- **Redis** caching with `go-redis/v9`
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
│   ├── /api                    # API server entry point
│   └── /worker                 # Payment job worker entry point
├── /internal
│   ├── /config                 # Configuration management
│   ├── /core                   # Shared value objects (Money, Address, Pagination, AppError)
│   │   └── /response           # Standard JSON response envelope
│   ├── /features               # Feature modules (vertical slices)
│   │   ├── /auth               # Authentication (register, login, JWT)
│   │   ├── /user               # User management & profiles
│   │   ├── /category           # Product categories
│   │   ├── /product            # Product catalog
│   │   ├── /inventory          # Stock management
│   │   ├── /cart               # Shopping cart
│   │   ├── /order              # Order lifecycle
│   │   ├── /payment            # Payment processing + worker jobs
│   │   ├── /shipping           # Shipment tracking
│   │   ├── /review             # Product reviews
│   │   ├── /promotion          # Coupons & promotions
│   │   ├── /wishlist           # User wishlists
│   │   ├── /notification       # User notifications
│   │   └── /dashboard          # Admin analytics
│   ├── /middleware              # HTTP middleware
│   ├── /platform               # Infrastructure layer
│   │   ├── /database           # PostgreSQL connection & transactions
│   │   ├── /cache              # Redis client
│   │   ├── /payment            # Payment gateway interface & implementations
│   │   ├── /validator          # Request validation
│   │   ├── /logger             # Structured logging
│   │   ├── /email              # Email service
│   │   └── /storage            # File storage
│   └── /server                 # HTTP server bootstrap & router
├── /db/migrations              # Database migration files (goose)
├── /mocks                      # Generated mocks (mockery v3)
└── ...
```

## Getting Started

### Prerequisites

- Go 1.26 or later
- PostgreSQL 16+
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

## Worker (Payment Job Processor)

The project includes a separate worker binary that processes payment jobs:

```bash
# Build worker
make build-worker

# Run worker
make run-worker
```

The worker polls the `payment_jobs` table and processes pending payments, updating order statuses and inventory accordingly.

Worker configuration via environment variables:

- `WORKER_INTERVAL` — Poll interval (default: 10s)
- `WORKER_BATCH_SIZE` — Jobs per batch (default: 10)
- `WORKER_LEASE_DURATION` — Job lease duration (default: 2m)
- `WORKER_CONCURRENCY` — Concurrent job processors (default: 5)

## Available Make Commands

```bash
make help             # Show all commands
make build            # Build all binaries (API + worker)
make build-api        # Build the API server
make build-worker     # Build the worker
make run              # Run the API server
make run-worker       # Run the worker
make dev              # Run with hot reload
make test             # Run tests
make test-coverage    # Run tests with coverage
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

Run all tests:

```bash
make test
```

Run tests with coverage:

```bash
make test-coverage
```

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
| `READER_DATABASE_URL`           | Read replica URL (optional)                    | —                                                         |
| `REDIS_HOST`                    | Redis host                                     | `localhost`                                               |
| `REDIS_PORT`                    | Redis port                                     | `6379`                                                    |
| `REDIS_PASSWORD`                | Redis password                                 | —                                                         |
| `REDIS_DB`                      | Redis database index                           | `0`                                                       |
| `JWT_SECRET`                    | JWT signing key                                | —                                                         |
| `JWT_ACCESS_TTL`                | Access token TTL                               | `15m`                                                     |
| `JWT_REFRESH_TTL`               | Refresh token TTL                              | `168h`                                                    |
| `JWT_ISSUER`                    | JWT issuer                                     | `ecommerce-api`                                           |
| `LOG_LEVEL`                     | Logging level (debug, info, warn, error)       | `info`                                                    |
| `LOG_FORMAT`                    | Log format (json, text)                        | `json`                                                    |
| `CORS_ALLOWED_ORIGINS`          | CORS allowed origins                           | `*`                                                       |
| `CORS_ALLOWED_METHODS`          | CORS allowed methods                           | `GET,POST,PUT,DELETE,OPTIONS`                             |
| `CORS_ALLOWED_HEADERS`          | CORS allowed headers                           | `Content-Type,Authorization,X-Request-ID,Idempotency-Key` |
| `CORS_MAX_AGE`                  | CORS max age (seconds)                         | `86400`                                                   |
| `WORKER_INTERVAL`               | Worker poll interval                           | `10s`                                                     |
| `WORKER_BATCH_SIZE`             | Worker jobs per batch                          | `10`                                                      |
| `WORKER_LEASE_DURATION`         | Worker job lease duration                      | `2m`                                                      |
| `WORKER_CONCURRENCY`            | Worker concurrent processors                   | `5`                                                       |
| `PAYMENT_GATEWAY`               | Payment gateway provider                       | `mock`                                                    |
| `PAYMENT_GATEWAY_URL`           | Payment gateway URL                            | —                                                         |
| `PAYMENT_GATEWAY_TIMEOUT`       | Payment gateway timeout                        | `10s`                                                     |
| `PAYMENT_GATEWAY_API_KEY`       | Payment gateway API key                        | —                                                         |
| `PAYMENT_WEBHOOK_SECRET`        | Payment webhook secret                         | —                                                         |

## Architecture

This template follows **Feature-Based Clean Architecture** (Vertical Slicing):

- Each feature (auth, user, product, order, etc.) is self-contained with its own handler, service, repository, and DTOs
- Dependencies flow inward (handlers → services → repositories)
- PostgreSQL repositories are embedded within each feature package
- Cross-feature dependencies use inline interfaces (consumer-defined) to maintain loose coupling
- Shared value objects (`Money`, `Address`, `Pagination`, `AppError`) live in `internal/core`

### Layer Responsibilities

| Layer        | Purpose                                                   |
| ------------ | --------------------------------------------------------- |
| `cmd`        | Application entry points (API server, worker)             |
| `config`     | Configuration management                                  |
| `core`       | Shared value objects, response envelope                   |
| `features`   | Feature modules (handlers, services, repos, DTOs)         |
| `middleware` | HTTP middleware (auth, RBAC, logging, recovery)           |
| `platform`   | Infrastructure (database, cache, payment, email, storage) |
| `server`     | HTTP server bootstrap & router                            |

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
