# Project Overview

Ecommerce API template in Go 1.26. It exposes REST endpoints under `/api` for auth, users, categories, products, inventory, cart, orders, payments, shipping, reviews, promotions, wishlists, notifications and an admin dashboard, and runs a separate worker process that drains payment, notification and order job queues. Storage is PostgreSQL (`pgx/v5`) with Redis (`go-redis/v9`); routing is stdlib `net/http` `ServeMux`.

The structure is the product here — other people copy this tree — so a boundary that a script can enforce always beats one that only code review can. `make check-boundaries` is that script, and `ARCHITECTURE.md` records why each rule exists and what it costs.

If this file disagrees with the code, the code wins: say so and fix the file.

## Repository Structure

- `cmd/api/` — API server binary (`server.Run()`)
- `cmd/worker/` — job worker binary (`worker.Run()`)
- `cmd/mockgateway/` — dev-only fake payment gateway; its `mockserver/` package is importable so `internal/server` can mount it in-process
- `internal/apperror/` — cross-module business sentinels (`ErrInsufficientStock`, `ErrCartEmpty`, …), each declared as a wrap of an `errs` kind; no feature deps
- `internal/app/` — the composition root: builds every `Service` and wires every cross-module port, and maps `config` values onto the platform option structs (`PoolOptions`, `ReplicaPoolOptions`)
- `internal/config/` — this application's infra env vars (`godotenv` + `envconfig`). Deliberately **not** under `platform/`: it names `APP_NAME`, `DB_*`, `WORKER_RESCUE_AFTER`, so it is the one config that is rewritten per project rather than copied
- `internal/server/` — `server.go` (`Run`), `router.go` (`NewRouter`, health, every route) and `auth.go` (`authMiddleware`, the one middleware that names a feature module)
- `internal/platform/` — generic infrastructure, no feature deps:
  - `cache/` — Redis client; `NewRedis` takes a `*redis.Options`
  - `database/` — pools (`PostgresOptions`), `TxRunner`, `PrimaryDB`/`ReplicaDB`, keyset and LIKE helpers
  - `errs/` — the five generic error kinds
  - `logger/` — `slog` setup, context attributes
  - `paging/` — cursor and offset pagination
  - `queue/` — insert-only River client and the transaction-aware `Insert`
  - `slug/`, `storage/`
  - `web/` — `Middleware`, `Chain`, `Router`; `web/request/` (`Bind`, `ParseUUIDParam`, the validator); `web/response/` (envelope, `HandleErr`, `CursorPage`); `web/middleware/` (CORS, logging, recovery, request ID, user context, `RequireRole`, `RateLimit`)
- `internal/testutil/` — shared dockertest harness (Postgres + Redis containers)
- `internal/worker/` — the jobs analogue of `server/`: owns the one working `river.Client`, its queue map and the order stale-sweep's `river.PeriodicJob`
- `internal/features/<feature>/` — the feature modules, plus two directories that are not features:
  - `checkout/` — a bounded context. Owns no table, no `domain/`, no store; orchestrates `order` and `payment` in one business transaction
  - `money/` — a shared kernel: the `Money` value object. No `Service`, no store, no routes, imports no other module
- `db/migrations/` — goose SQL migrations
- `db/seeds/data.sql` — seed data, applied by `make seed`
- `db/OWNERSHIP.md` — table to owning module, parsed at run time by `make check-boundaries`
- `test/e2e/` — cross-module saga tests through the real router
- `scripts/check-boundaries.sh` — the architectural checks; `scripts/boundaries_test.go` proves each one still matches something

### Inside a module

A module is one flat package plus an `adapter/` directory. There is no `usecase/`, no `module.go` and no `Module` type.

```text
internal/features/<feature>/
  service.go         one exported Service plus New. No Deps struct: New takes
                     dependencies as positional parameters, so a forgotten one
                     is a compile error rather than a nil field
  repository.go      the storage port adapter/postgres satisfies
  ports.go           every cross-module port this module consumes, one file
  contract.go        the struct types another module may name
  config.go          this module's own env vars
  domain/            aggregate types and rules -- private to the module
  channel.go         the outbound Channel port (notification only)
  queue.go           the outbound job-enqueue port, named Queue
  service_test.go    mock-driven tests, package <feature>
  mocks_test.go      mockery output, in-package
  adapter/
    postgres/        SQL adapter
    http/            handlers plus their wire types
    redis/           the store behind a cache port (user only)
    gateway/         the outbound Gateway port and its implementations (payment only)
    channel/         the Channel port's log implementation (notification only)
    jobs/            job args, InsertOpts and a river.Worker; the only place in
                     the module that names river
```

**The subpackage tree is deliberately non-uniform.** A module gets an `adapter/postgres` only if it has SQL, an `adapter/http` only if it has a route, a `ports.go` only if it reaches outside itself, a `contract.go` only if a struct of its own crosses a port. `auth` keeps no store at all and asks `user` for everything through one port. `user` is the only module with two backing stores. Do not tidy the tree into uniformity, and do not add a pass-through adapter package to fill a slot. Read `ls internal/features/<feature>/` for the module in front of you:

```bash
ls -1 internal/features                      # every module directory
ls internal/features/*/service.go            # which have a Service
ls -d internal/features/*/adapter/http       # which have routes
grep -h '^type .* interface' internal/features/*/ports.go
```

Inside `adapter/http`, files split by handler role: unqualified `handler.go` is the public or authed handler, `admin_handler.go` and `webhook_handler.go` are the qualified exceptions. Wire types split the same way — `request.go`/`response.go` for public shapes, `admin_request.go`/`admin_response.go` for admin-only ones — and a type shared between roles goes in the unqualified file, not in both. No `routes.go` exists anywhere under `internal/features/`: every URL lives in `internal/server/router.go`.

## Build & Development Commands

All verified against the `Makefile`; `make help` lists them with descriptions.

```bash
# Install deps + air, golangci-lint, goose; copy .env.example
make setup

# Build binaries
make build              # API + worker
make build-api
make build-worker
make build-mockgateway  # dev-only fake gateway

# Run
make run                # API server
make run-worker
make dev                # hot reload via air

# Test
make test               # go test -v -race -count=1 -timeout 5m -cover ./...
make test-one NAME=X    # go test -run X across ./... with .env loaded
make test-coverage      # ./internal/... ./test/... -> coverage.out + coverage.html
make test-clean         # remove the shared postgres + redis test containers
make mocks              # regenerate every mocks_test.go from .mockery.yml

# Check
make check-boundaries   # the architectural checks; prints "Boundaries OK"
make lint               # golangci-lint run ./...
make vet
make fmt                # go fmt ./... && gofmt -s -w .
make vuln               # govulncheck
make tidy
make clean

# Pipelines
make all                # fmt -> vet -> check-boundaries -> lint -> test -> build
make ci                 # deps -> fmt -> vet -> lint -> test
```

Two things about those pipelines: **`make ci` does not run `check-boundaries`, `make all` does** — rely on `make all`, or run `make check-boundaries` explicitly. And `make test` runs `./...` while `make test-coverage` globs `./internal/... ./test/...`, so a new top-level test directory is picked up by the first and silently skipped by the second.

Database commands need the goose and river CLIs (`make migrate-install` installs both; the Makefile expects them under `$(go env GOPATH)/bin`). They build `DATABASE_URL` from `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE`, all overridable:

```bash
make migrate-up  migrate-down  migrate-down-all  migrate-status  migrate-version
make migrate-create name=add_something
make migrate-jobs      # River's own migrations -- the worker cannot start without them
make db-create  db-drop  seed
make docker-up  docker-dev  docker-down  docker-logs  docker-build  docker-clean
```

## Code Style & Conventions

- **Language and libraries.** Go 1.26. stdlib `net/http` `ServeMux` — do not add a third-party router. `encoding/json` for JSON, `log/slog` for logging, `go-playground/validator/v10` for validation, `godotenv` + `kelseyhightower/envconfig` for config.
- **Formatting.** `gofmt -s`, enforced by `make fmt` and golangci-lint. Import groups: stdlib, blank line, third-party, blank line, local (`github.com/residwi/go-api-project-template/...`).
- **Packages** are short singular nouns (`user`, `product`, `cart`).
- **Errors.** Five generic kinds in `internal/platform/errs` (`ErrNotFound`, `ErrConflict`, `ErrBadRequest`, `ErrUnauthorized`, `ErrForbidden`); cross-module business sentinels in `internal/apperror`. Add context with `fmt.Errorf("%w: ...", errs.ErrBadRequest)`.
- **A business sentinel is declared as a wrap of a generic kind, never a bare `errors.New`.** `response.HandleErr` matches only the five `errs` kinds, so a sentinel that unwraps to none of them becomes a 500 no caller can distinguish from a database outage. The declaration is the only place this can be got right: `apperror.ErrOrderCharging` is a 409 because it wraps `errs.ErrConflict`, not because a transport file says so. `errors.Is(err, errs.ErrConflict)` therefore matches every sentinel wrapping it — match the business sentinel when you mean the business case.
- **No stdlib `log`** anywhere (`depguard` denies it across `$all`), and **no `slog.Any`** (`forbidigo` denies the identifier). Every attribute names its type: an error is `slog.String("error", err.Error())`, a recovered panic is `slog.String("panic", fmt.Sprint(rec))`.
- **One `Service` per module, and its methods carry the verb.** No `Execute`. No module-name stutter — `cart.Get`, not `cart.GetCart`. An entity module implies its object (`category.Create`, `order.Get`); a process module names it (`checkout.PlaceOrder`, `checkout.CancelOrder`). `order.Place` rather than `Create`, because the operation locks the cart, validates it, reserves inventory and a coupon, writes the order, charges and clears the cart. `GetForUser` beside `Get` marks the method that performs an ownership check.
- **A cross-module port is declared in the consuming module's own `ports.go`; the producer never publishes it.** One port per producer, not one per capability: a module declares one interface per other module it consumes, holding every method it needs from it. Two mechanisms satisfy a port without an adapter, and `internal/app` is the one place either is used — **name-match** (the producer's value already has a method named what the port asks for) and a **`contract.go` type** (when what crosses is a struct). There is no shared ports package.
- **An `adapter/http` port is named for the role it plays, never for the pattern.** `CartManager`, `ProductReader`, `WebhookProcessor`, `Reporter` — never `UseCase`, and never `Service` either, since the port is a subset of what the module's `Service` offers. Role naming is what lets two ports coexist in one package when routes split by caller role. The `Handler` field holding the port is `service`, and so is the constructor parameter; constructors are `NewHandler`, `NewAdminHandler` and `NewWebhookHandler`.
- **Wire types live in the module's own `adapter/http`.** A `json` tag belongs nowhere else, `json:"-"` belongs nowhere at all, and no file may be named `dto.go`. All three are machine-checked.
- **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope is `order`, `payment`, `product`, `cart`; `promotion` and `dashboard` stay on `int64` for reasons `ARCHITECTURE.md` records. `Money` carries no `json` tag and implements no `sql.Scanner` — each adapter maps it explicitly, because wire shapes genuinely differ per endpoint. No float constructor, no `Div`.
- **A `Service` runs no SQL and holds no pool.** Every read and write goes through the module's own `Repository`; `adapter/postgres` owns the pool and reaches it with `database.PrimaryDB(ctx, db)` or `database.ReplicaDB(ctx, db)` — both return the context's transaction if there is one, and `ReplicaDB` falls back to `Primary` when no replica is configured. Use `ReplicaDB` only for read-only methods.
- **Services take `database.TxRunner`, never `*pgxpool.Pool`.** A service composes several repository calls into one unit of work through it, and the transaction propagates to every repository it touches — its own and other modules' — through `ctx`. A module that opens no transaction takes no runner at all. `internal/app` constructs the adapters and threads one `database.DB` value through them, so the pool never reaches a `Service`.
- **Repository reads use `pgx.CollectRows`**, never a hand-rolled `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and build keyset predicates with `database.KeysetCursor()`.
- **Handlers use the shared helpers.** Decode and validate with `request.Bind[T](w, r)`, read the caller with `middleware.RequireUser(w, r)`, return errors through `response.HandleErr`. Do not hand-roll decode/validate or auth-context blocks. `Bind` owns the validator — there is one shared `validator.New()` in the tree and a handler carries none; register a custom tag beside it, not per handler.
- **Order status changes only through `order.Service.Apply`.** Every guarded transition is a named `domain.Transition` in `internal/features/order/domain/state.go`, carrying the statuses it may be applied from and a stock effect the adapter reads through `DeductsStock()`/`ReversesStock()`. Other modules depend on intent methods on their own port (`payment.Orders.MarkPaid`, `shipping.Orders.MarkShipped`), and the guard itself is the `WHERE status = ANY(...)` in `Repository.Apply`. Never write an ad-hoc from/to status list at a call site.
- **Inventory reversal goes through `inventory.Service.Restore`.** It decides whether that means releasing a reservation or restocking deducted goods; callers supply the order's prior `StockState`, never the mechanics. `StockState` and `StockStateOf(deducted bool)` live in `inventory/contract.go` — do not reintroduce a private per-module copy of that conversion.
- **Background jobs run on River.** `internal/platform/queue` holds an insert-only client (`NewInsertClient`) and `Insert`, which type-asserts `database.PrimaryDB(ctx, db)` to `pgx.Tx` and uses `InsertTx` when the caller is inside a transaction — without that, an enqueue survives a business write that rolled back. Each module that has a job keeps an `adapter/jobs` package holding the args type, its `InsertOpts()` and a `river.Worker`, and that is the only place in the module naming `river`. The outbound port is `Queue`, unqualified, the way `repository.go` declares `Repository`. Never hand-roll a ticker, lease or poll loop: River owns polling, the leased claim, bounded concurrency, per-job timeouts and maintenance.
- **New config invariants go in the owning type's own loader** — infra ones in `Settings.validate()`, module ones inline in that module's `LoadConfig`. Misconfiguration must abort boot rather than surface later as a runtime error. Do not guard per use site.
- **Request-scoped log attributes are named once, at the edge.** `logger.WithAttrs(ctx, ...)` stashes them and `logger.ContextHandler` merges them into every record below, so no function grows a parameter to carry `request_id`. An attribute may only be named at an edge that owns exactly one value — `order_id` stays at the call site because a command loops over batches. Naming an attribute at two points on one path emits the key twice; slog does not deduplicate.
- **Exported `type`, `var` and `const` declarations come before unexported ones in a file.** Two exemptions: a `var _ Iface = (*T)(nil)` compile assertion stays adjacent to the type it asserts about, and a context-key type stays beside the functions that read it. The rule covers declarations, not every identifier: an unexported type carrying an exported method sits below the exported type it serves, which `funcorder`'s `struct-method` check requires. `funcorder.function: true` orders free functions too, and `make lint` catches it — but golangci-lint caches per package, so run `golangci-lint cache clean` before trusting a clean lint after a toolchain upgrade.
- **No comments in Go source, except directives and a comment that names a specific regression or a non-obvious constant.** Directives always stay — every `//nolint:` (with its justification on the same line) and every `//go:`. What survives beyond them today is `internal/features/checkout/service.go`'s note above `if created && order.Total.Amount > 0` (without it the `created &&` guard reads as redundant, and deleting it reintroduces a double-charge bug) and `internal/features/auth/service.go`'s notes on `dummyPassword` and `maxPasswordBytes` (bcrypt limits that the code cannot state itself). A comment earns its place by naming what breaks without it, not by explaining what the line already says.
- **Prefer duplication over an abstraction that does not quite fit.**
- **Commit messages** use conventional-commit prefixes, matching surrounding history (`refactor(cart): …`, `docs(db): …`, `test(e2e): …`).

## Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│  cmd/api                          cmd/worker                 │
│  server.Run()                     worker.Run()               │
│      │                                │                      │
│  internal/server                  internal/worker            │
│  NewRouter: every URL,            one river.Client,          │
│  route groups, authMiddleware     queue map, periodic sweep   │
│      │                                │                      │
│      └──────────────┬─────────────────┘                      │
│                     │                                        │
│              internal/app                              │
│      builds every Service, wires every port                  │
│                     │                                        │
│   ┌─────────────────┴──────────────────────────────┐         │
│   │  internal/features/<feature>                    │         │
│   │                                                │         │
│   │   adapter/http ──► Service ──► Repository      │         │
│   │   (wire types)      │  ▲        (port)         │         │
│   │                     │  │            │          │         │
│   │            ports.go │  │ contract.go│          │         │
│   │        (what it     │  │ (what it   │          │         │
│   │         consumes)   │  │  publishes)│          │         │
│   │                     ▼  │            ▼          │         │
│   │              another module's  adapter/postgres│         │
│   │              root package      adapter/jobs    │         │
│   └────────────────────────────────────────────────┘         │
│                     │                                        │
│              internal/platform                               │
│   database │ cache │ queue │ web │ errs │ logger │ paging    │
│                     │                                        │
│              PostgreSQL + Redis                              │
└──────────────────────────────────────────────────────────────┘
```

### Machine-checked boundaries

`make check-boundaries` runs `scripts/check-boundaries.sh` and fails the build on any of these. The numbering has gaps because checks 5 and 7 were retired and renumbering would falsify every by-number citation in this file, `ARCHITECTURE.md`, `db/OWNERSHIP.md` and the script itself.

1. **`check_wire_tags`** — a `json` tag lives only in a module's own `adapter/http`. `internal/features/<feature>/http/` is not exempt, and neither is `internal/server`. `json:"-"` may not appear anywhere under `internal/`, and no file may be named `dto.go`. `internal/platform/` is exempt by location (the response envelope, the `envconfig` tags); `internal/features/payment/adapter/gateway/gateway.go` is allowlisted by name, being the external gateway's wire contract rather than ours.
2. **`check_ownership_doc`** — `db/OWNERSHIP.md` has no duplicate row, no row for a table no migration creates, and no table without an owning row. The list is parsed out of the document at run time, so the document and the check cannot drift.
3. **`check_table_ownership`** — a module's SQL only names tables it owns. It scans every non-test `.go` file in the module, not just `adapter/postgres`, and a CTE named after a real table is a violation rather than an exemption. `dashboard` is exempt by name, being a reporting read model. Change ownership in `db/OWNERSHIP.md`; there is no list in the script to keep in step.
4. **`check_cross_module_imports`** — a module may import another module's root package, which is its published surface, and nothing deeper. `domain/` and every adapter stay private. Only the wiring layer (`app`, `server`, `worker`) may reach inside a module. One per-importer exemption exists: `checkout` alone may import a module's `domain/`, because `order.Service.Place`'s signature is written in `orderdomain` types.
5. _Retired — `check_sibling_slice_imports`._
6. **`check_transport_direction`** — a module may not import `internal/server`, with no exemption. It catches a `Service` returning a transport type and a module registering its own routes; either would make every binary constructing that module link HTTP, including the worker, which serves nothing. A module that must describe something the transport also describes puts the type in its own `contract.go`.
7. _Retired — `check_contract_leaf`._
8. **`check_platform_leaf`** — nothing under `internal/platform` may import a local package outside `internal/platform`, so that `cp -r internal/platform` into a fresh module compiles with no edits. The check matches every import of this repository's own code and subtracts what is allowed, rather than naming forbidden trees, so a tree added later is covered on the day it appears. `internal/testutil` is the one exemption, which is why the copy property holds for `go build` and not `go test`.

`scripts/boundaries_test.go` plants a probe file in a real module for each check and asserts the script reports it, and probes from the other side too, so an exemption that has stopped matching anything fails a test instead of printing `Boundaries OK`. Run it with `go test ./scripts/`.

Read "What it does not catch" in `db/OWNERSHIP.md` before trusting a green run. In short: table names must be string literals, `_test.go` files are skipped by checks 1 and 3 but deliberately in scope for check 4, `dashboard` is exempt wholesale, ownership is per table so column coupling is invisible, every check walks `internal/` only — `cmd/` and `test/` are outside all of them — and none of the checks is a compiler; they are all greps.

### Conventions the checks cannot see

- **A module imports another module's root package and nothing deeper**, and that means `payment` _can_ see `order.Place`. Nothing stops a module calling a sibling method no port of its own declares, and no check can tell the difference. Declare the interface the consumer needs in the consumer's own `ports.go` and wire it in `internal/app`; do not reach for a sibling's method because the import already compiles.
- **The arrow runs the other way for URLs.** The transport imports modules and a module names no URL: every route lives in `internal/server/router.go`, mounted on the route groups that same function builds. A module supplies a handler with exported route methods — exported precisely so `router.go`, a different package, can name them — and the transport decides the verb, the path and the group.

### Data flow: place an order

1. `POST /api/checkout/orders` lands on `checkout`'s handler.
2. `checkout.Service.PlaceOrder` calls `order.Service.Place` through its own `Orders` port: one transaction that checks idempotency, locks and validates the cart, reserves inventory, writes the order and its items, reserves a coupon and clears the cart.
3. `checkout` then charges through its `Payments` port — `payment.Service.Charge` writes the payment row and calls the gateway.
4. Terminal gateway results arrive as a webhook on `payment`'s `webhook_handler.go`; a refund is enqueued as a River job in `payment/adapter/jobs` and drained by `internal/worker`.
5. Order status only ever moves through `order.Service.Apply`, so stock deduction and reversal follow the transition's own stock effect.
6. `order/adapter/jobs.ExpireStaleWorker` runs as a `river.PeriodicJob`, re-derived on every client start, and sweeps stale `awaiting_payment` orders. A long outage yields one sweep on restart rather than a backlog.

`ARCHITECTURE.md` records the decision behind each of these and what it costs.

## Testing Strategy and Rules

- **Framework.** `testing` + `stretchr/testify`. `require` when the test cannot continue without the value, `assert` for soft checks.
- **Mocks are generated** by mockery v3 from `.mockery.yml`, **in-package**, as `mocks_test.go` beside the interface they mock. A `_test.go` file never enters its package's importable `GoFiles`, so a mock is private to that package, cannot cycle back, and keeps every `Mock*` name out of the module's exported API. `.mockery.yml` is one recursive rule rooted at `internal/` with `all: true` and no per-interface list. Run `make mocks`; never hand-edit the output. Use the expecter API, never `On`:

  ```go
  repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)   // correct
  repo.On("GetByID", mock.Anything, orderID).Return(existingOrder, nil)      // wrong
  ```

  This is also why an `adapter/http` handler declares its own narrow port locally: mockery cannot write a private mock into a package that does not declare the interface.

- **Tests live in the package they test**, except where an import cycle or a deliberate outside-view preference puts them in an external `_test` package — `test/e2e`, `scripts/`, and a handful of others. A module's `service_test.go` is always `package <feature>` and its `handler_test.go` always `package http`, so one file can hold both route-level tests through a mux and direct tests of unexported mappers. Put a new test where its access requires, then name the file for that.
- **Where a test belongs.** Anything only the database can prove — recursive CTEs, keyset pagination, unique constraints — goes in `adapter/postgres/repository_test.go` (or `adapter/redis/cache_test.go`) against a real container. Anything a mock can express — a `Service`'s reaction to a value, an error branch — goes in that module's `service_test.go`. A saga spanning tables no single module owns goes to `test/e2e/`, driven through the real `server.NewRouter`. No module starts its own container, and there is no `test/integration` directory: `go test ./...` runs package binaries concurrently, and collapsing per-package tests into one package would serialise them.
- **A handler test proves the handler, not the URL.** Every test under `internal/features/*/adapter/http/` builds its own `web.NewRouter(mux).Group(...)` and picks its own prefix — several use paths production has never served. `internal/server/routes_access_test.go` is the only place that enumerates the real table: `allRoutes` is a hand-written list of `method<TAB>path` lines, `publicRoutes` names the routes that must answer an anonymous caller, and an `/api/admin/` prefix must give a non-admin a 403. `web.Router` records nothing, so a route missing from `allRoutes` is simply never probed. **Adding a line to `publicRoutes` opens a route to the internet** — that edit wants a second reader.
- **Docker is required.** No build tags, no short mode. `internal/testutil` starts two long-lived containers by fixed name (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to whichever already exists; `make test-clean` removes them. Every package binary races for the same container name, and the loser polls until the winner's container is running with a bound port.
- **Postgres databases are per module, created once under an advisory lock, and never dropped.** `MustStartPostgres(dbName)` creates and migrates `dbName` the first time any caller asks for it — the lock covers the migration, so a later caller always finds the latest schema — and every later caller just connects. `grep -rn 'MustStartPostgres(' --include='*_test.go' internal` is the live name mapping.
- **`ResetDB` is safe only for a package that owns its database outright.** It takes a `*pgxpool.Pool`, not a package name, so nothing stops a new caller inside a module adding it — check before copying a `setup` helper that calls it. It never truncates `goose_db_version` or `river_migration`: migrations run on every `MustStartPostgres`, so clearing either version table would send the next caller over a schema that already exists.
- **`MustStartRedis(dbIndex)` takes an index you pick by hand.** Claimed today: 0 (`platform/cache`), 1 (`platform/web/middleware`), 3 (`internal/server`), 5 (`test/e2e`), 6 (`modules/user/adapter/redis`). Nothing enforces a claim — a collision compiles, passes review and fails as a flake in an unrelated package — so `grep -rn 'MustStartRedis(' --include='*_test.go' .` is the authority, and update this list in the commit that takes an index.
- **`t.Parallel()` is mandatory everywhere except in a package that owns a database or a Redis index**, where everything shares one connection and `ResetDB` truncates every table. Those packages are excluded from `paralleltest` wholesale in `.golangci.yml` — per package, never per file, because a parallel sibling gets its rows deleted mid-assertion even when it never calls reset itself. Most alternatives in that `path:` regex are anchored to the repo root and die silently when the directory they name moves, so check them against `git ls-files` after any structural change; `/postgres/` and `/redis/` are unanchored and follow their directories. If you add a test package that claims a database or a Redis slot, add it to that exclusion in the same commit.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest, a descriptive name, its own setup. No monolithic tests. Duplication in setup is cheaper than a helper that hides expectations: a `newTestService(t)` returning mocks is fine, one that also sets up mock expectations is not.
- **Compare whole objects, not field by field** — `assert.Equal` on the full struct or slice. For JSONB round-trips use `assert.JSONEq`, since Postgres normalises whitespace.
- **Test behaviour, not wiring.** Verify a returned value, an error, or a side effect.
- **Order a test file so the tests come first.** Package-level `var`s and `TestMain` at the top, then every `func TestXxx`, then stub types with their methods grouped under them, then plain helpers last. `funcorder` only orders methods against their struct, so the rest is on you.
- **Keep tests fast.** Use `bcrypt.MinCost` for password hashes (`DefaultCost` costs ~250ms per hash) and group tests that exercise the real `Register` path. Reach for `testing/synctest` before hand-rolling around real time for ticker- or timeout-driven code — but note it cannot wrap a `pgxpool` acquire, so a test holding a real pool must shrink intervals and timeouts instead. Give intentionally-broken clients short timeouts (`MaxRetries: 0`, `DialTimeout: 200 * time.Millisecond`) so error paths fail in milliseconds.

## Security & Compliance

- Secrets come from env vars or a gitignored `.env`. Never commit real secrets. `.env.example` is the exhaustive list of supported variables; the README's table is a curated subset.
- JWT auth with configurable expiry, bcrypt password hashes, RBAC through role middleware.
- **Middleware lives in two places, and the line is whether it names a feature module.** Check 8 answers that by reading imports rather than by judgment: nothing under `internal/platform` may import `internal/features`. `internal/platform/web/middleware` holds panic recovery, request-ID injection, request logging, CORS, the user context (`UserContext`, `SetUserContext`, `GetUserContext`, `RequireUser`), `RequireRole` and rate limiting. `internal/server/auth.go` holds `authMiddleware` alone, because it names `auth.ClaimsView` and `user.AccountStatus`. A new compression or timeout middleware goes in platform; one that needs a module's type goes beside `authMiddleware`. A middleware needing an import platform may not have is the answer, not a reason to widen check 8.
- **Field exposure is controlled by DTO omission, not by `json:"-"`.** Adding a field to a response means naming it in a wire type deliberately. The failure mode this leaves is naming the _wrong_ mapper: public and admin response mappers live in separate files in the same package, and a handler can call either and compile. Check which mapper an admin route uses before changing it.

## Agent Guardrails

- Never hand-edit a generated `mocks_test.go` — regenerate with `make mocks`.
- Never commit `.env`, secrets or API keys.
- Run `make check-boundaries`, `make vet` and `make test` before calling a change complete. `make all` does all three plus lint and build.
- Do not add a third-party router.
- Do not suppress a lint or vet finding with `//nolint` without a justification comment on the same line. The expected form is `//nolint:gocognit,funlen // one order write: idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction`.
- Do not make the adapter subpackage tree uniform, and do not add a pass-through adapter package to fill a slot.
- Backward compatibility is explicitly **not** a goal. API shapes may change where a better design demands it — but say so when they do.
- **Adding a module:** create `internal/features/<feature>/` in the shape above, add a row per owned table to `db/OWNERSHIP.md`, wire it into `internal/app/app.go` (one line to build it, one field on `Services`), mount its routes in `internal/server/router.go`, then run `make check-boundaries` — a new module with an `adapter/postgres` and no ownership row fails it by design.
- **Adding one route touches three files:** the module's `adapter/http` for the handler, `internal/server/router.go` for the URL, and `allRoutes` in `internal/server/routes_access_test.go` — without the last one the route is never probed. A second line in `publicRoutes` is needed only when the route must answer anonymous callers.

## Further Reading

- [README.md](README.md) — endpoint reference and quick start.
- [ARCHITECTURE.md](ARCHITECTURE.md) — the decisions behind this shape, what each one costs, and what it makes hard.
- [db/OWNERSHIP.md](db/OWNERSHIP.md) — table to owning module, and what the ownership check cannot see.
- [db/migrations/](db/migrations/) — goose SQL migrations.
- [.env.example](.env.example) — every supported environment variable.
- [.mockery.yml](.mockery.yml), [.golangci.yml](.golangci.yml) — mock and lint configuration.
