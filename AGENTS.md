# AGENTS.md

Orientation for agents and humans in this repo. Describes tree as it actually is, commands that actually exist, and — most useful — which rules **machine-checked** vs which only convention.

Three docs carry the reasoning; this one no duplicate:

- **`ARCHITECTURE.md`** — thirteen decisions that shaped this codebase, fifteen things it deliberately not do, each with cost.
- **`ARCHITECTURE-LIMITATIONS.md`** — what those decisions make hard or impossible, and what you must build to get past each. Read before proposing feature that crosses module boundary.
- **`db/OWNERSHIP.md`** — which module owns which table, parsed at run time by `make check-boundaries`, plus what that check cannot see.

If this file ever disagree with code, code wins — say so and fix file.

## What this is

Go 1.26 ecommerce API template. REST endpoints under `/api` for auth, users, categories, products, inventory, cart, orders, payments, shipping, reviews, promotions, wishlists, notifications, admin dashboard, plus separate worker process draining payment and notification job queues. PostgreSQL via `pgx/v5`, Redis via `go-redis/v9`, routing on stdlib `net/http` `ServeMux` — no third-party router.

Structure is product. Others copy template, so boundary compiler or CI can enforce beats boundary code review must.

## Repository structure

```text
cmd/api/                  API server binary
cmd/worker/               payment + notification job worker binary
cmd/mockgateway/          dev-only fake payment gateway binary
  mockserver/             its handlers, importable so transport/http can mount them in-process
internal/
  apperror/               error vocabulary (ErrNotFound, ErrBadRequest, ...); no feature deps
  money/                  the Money value object; no feature deps
  config/                 godotenv + envconfig; Load() then validate()
  bootstrap/              cross-feature adapters + constructors for cross-dependent services
  transport/http/         server.go, router.go, middleware/, response/
  platform/               generic infrastructure, no feature deps:
                          cache/ database/ jobs/ logger/ paging/ slug/ storage/ validator/
  testhelper/             shared dockertest harness (Postgres + Redis containers)
  modules/<feature>/      14 feature modules (see below)
db/migrations/            goose SQL migrations
db/seeds/data.sql         seed data, applied by `make seed`
db/OWNERSHIP.md           table -> owning module, read by the boundary check
test/e2e/                 cross-module saga tests through the real router
scripts/check-boundaries.sh   the architectural checks
```

`internal/modules/` holds the **14 features** — `auth cart category dashboard
inventory notification order payment product promotion review shipping user
wishlist`. Everything else under `internal/` is infrastructure —
`apperror bootstrap config money platform testhelper transport`.
`scripts/check-boundaries.sh` derives feature list structurally, reading directory names under `internal/modules/`, so adding feature enough to enrol it in boundary checks; no denylist to remember.
Being infrastructure exempts directory from checks 2 and 3 _ownership_ questions, not from check 3 itself: only wiring layer — `bootstrap` and `transport`, script's `WIRING_DIRS` — may import feature's adapter, so `internal/platform/` importing `internal/modules/product/postgres` still fails.

### Inside a feature

Feature holds its domain types, its service, its repository _interface_, and ports it needs from other features. Adapters are subpackages.

```text
internal/modules/order/
  model.go        domain types
  params.go       input structs for service methods
  ports.go        interfaces order needs from other features
  repository.go   the Repository interface order's own storage must satisfy
  service.go      Service struct and its methods
  transition.go   order's state machine (feature-specific file)
  address.go      feature-specific file
  postgres/       the Postgres repository — the only place order's tables are named
  http/           routes.go plus one file per handler role: order's is split into
                  handler.go and admin_handler.go, each owning the
                  request/response DTOs and mapping for its own handlers,
                  and each with a _test.go beside it
```

Every feature has `model.go`, `service.go`, `repository.go` except `auth`, which has no storage of own. There is **no** `handler.go` or `routes.go` at feature root — those live in `internal/modules/<feature>/http/`. A `dto.go` belongs nowhere at all: check 1c refuses that filename **anywhere** under `internal/`, `http/` included. Wire types live in handler file that serialises them.

Ports usually in `ports.go`, but two features name file after module they depend on instead: `internal/modules/category/product.go` declares `ProductCounter`, and `internal/modules/product/inventory.go` declares `InventoryReader` and `InventoryRegistrar`. Either fine. Rule about _who declares the interface_ (consumer), not filename.

**Subpackage tree deliberately non-uniform. Do not tidy it into uniformity.** Feature has subpackage only where adaptation needed:

| Feature      | Subpackages                                                       |
| ------------ | ----------------------------------------------------------------- |
| `payment`    | `postgres/ http/ stripe/ midtrans/ mock/ worker/`                 |
| `auth`       | `http/` only — no storage; asks `user` via `auth.UserProvider`    |
| `dashboard`  | `postgres/ http/` — but owns no table (see reporting carve-out)   |
| `user`       | `postgres/ http/ redis/` — only feature with second backing store |
| the other 10 | `postgres/ http/`                                                 |

`notification` has no `worker/` package because `notification.Service` satisfies `jobs.Processor` direct. That absence is the lesson — `ARCHITECTURE.md` decision 4 — not omission to fix. `user/redis/` is positive case of same rule: subpackage exists where feature has that kind of backing store, and `user` only feature caching, so `ls internal/modules/user/` still tells truth about which features do. Feature declares one port per store — `repository.go` for Postgres, `cache.go` for cache — and gets one adapter subpackage per port: `user.Repository` pairs with `postgres/`, `user.StatusCache` with `redis/`. That adapter requires Redis 8.0 or later; built on `HSETEX`, which sets hash fields and their expiry in one atomic command, and that command not exist on earlier Redis. There are 13 packages named `postgres`, 14 feature packages named `http`, and one named `redis`, which is why `internal/transport/http/router.go` needs 28 aliased adapter imports.

Inside `http/`, file split by **handler role**, not endpoint. Unqualified name = default handler; `admin_` and `webhook_` = qualified exceptions. Every feature holds subset of exactly these eight names and nothing else:

| File                      | Package | Holds                                                                                 |
| ------------------------- | ------- | ------------------------------------------------------------------------------------- |
| `routes.go`               | `http`  | `RouteDeps` and `RegisterRoutes` only — no DTOs, no logic                             |
| `handler.go`              | `http`  | default (public or authed) handler, its DTOs and mappers                              |
| `handler_test.go`         | `http`  | its route-level tests, driven through mux, plus leak tests for its unexported mappers |
| `admin_handler.go`        | `http`  | admin handler, where routes split by caller role                                      |
| `admin_handler_test.go`   | `http`  | its route-level tests plus leak tests for its unexported mappers                      |
| `webhook_handler.go`      | `http`  | `payment` only — gateway callback                                                     |
| `webhook_handler_test.go` | `http`  | its route-level tests                                                                 |

Counted across `internal/modules/*/http/`: `routes.go` ×14, `handler.go` and
`handler_test.go` ×11, `admin_handler.go` and `admin_handler_test.go` ×10,
`webhook_handler.go` and `webhook_handler_test.go` ×1.

Three features have **no** `handler.go`, and that is naming rule working, not omission: `payment`, `dashboard`, `inventory` register every route on admin group, so their only handler is an `adminHandler`. If feature's `http/` has no `handler.go`, it has no non-admin surface — fact worth reading off `ls`.

**Tests live in package they test, except where import cycle forbids it.** `handler_test.go`, `admin_handler_test.go`, `webhook_handler_test.go` are `package http`, holding both route-level tests driven through mux and leak tests calling unexported mappers (`toProductResponse`, …) direct — tests that stop domain field reaching unauthenticated response body. In-package permits white-box testing without preventing black-box testing, so one file now does both; that dissolves old constraint that split them, which is why separate leak-test file per feature is gone — contents moved beside implementation file declaring what they test.

Two carve-outs remain, both cycles not preferences, and together they are the whole exception: 10 external test files, no more. Service tests no longer among them: mocks generate in-package, so mock no longer imports package it mocks, and every feature-root `_test.go` file — `service_test.go` included — is `package <feature>`.
`test/e2e` (9 files, `package e2e_test`) imports concrete adapters —
`internal/modules/*/postgres`, `internal/bootstrap`,
`internal/transport/http` — across every module the saga touches; no single feature package can own that without becoming dependent of its siblings, which `make check-boundaries` forbids. `internal/testhelper/txrunner_test.go` (1 file, `package testhelper_test`) asserts `database.TxRunner` satisfied from outside `testhelper`, which cannot import `platform/database` itself — `platform/database`'s own in-package tests import `testhelper` for `MustStartPostgres`, so dependency can only run other way in external file. Go's own standard library draws per-file line same way: `net/http` ships 19 in-package test files (`package http`) beside 18 external `package http_test` files in same directory, choosing per file by access test needs, not one package-wide policy. Put new test where its access requires, then name file for that.

## Commands

All verified against `Makefile`; `make help` lists them with descriptions.

```bash
make setup             # deps + install air, golangci-lint, goose; copy .env.example
make deps              # go mod download && go mod verify

make build             # build-api + build-worker
make build-api
make build-worker
make build-mockgateway # the dev-only fake gateway
make run               # go run ./cmd/api
make run-worker
make dev               # hot reload via air

make test              # go test -v -race -count=1 -timeout 5m -cover ./...
make test-coverage     # ./internal/... ./test/... -> coverage.out + coverage.html
make test-clean        # remove the shared postgres + redis test containers

make check-boundaries  # the architectural checks; prints "Boundaries OK"
make lint              # golangci-lint run ./...
make vet
make fmt               # go fmt ./... && gofmt -s -w .
make vuln              # govulncheck
make tidy
make clean

make all               # fmt -> vet -> check-boundaries -> lint -> test -> build
make ci                # deps -> fmt -> vet -> lint -> test
```

Two things about those last two worth knowing:

- **`make ci` does not run `check-boundaries`; `make all` does.** If you rely on one command before calling work finished, use `make all`, or run `make check-boundaries` explicit.
- **`make test` runs `./...` while `make test-coverage` globs
  `./internal/... ./test/...`.** New top-level test directory picked up by first, silently skipped by second.

DB commands need goose CLI (`make migrate-install`; Makefile expects it at `$(go env GOPATH)/bin/goose`). They build `DATABASE_URL` from `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE`, all overridable:

```bash
make migrate-up  migrate-down  migrate-down-all  migrate-status  migrate-version
make migrate-create name=add_something
make db-create  db-drop  seed
make docker-up  docker-dev  docker-down  docker-logs  docker-build  docker-clean
```

## Architectural rules

### Machine-checked

`make check-boundaries` runs `scripts/check-boundaries.sh` and fails build on any of these. This part worth memorising — these rules you cannot violate quiet.

1. **No `json` tag outside `internal/modules/<feature>/http/`.** Domain models carry no transport concerns; every endpoint owns its request DTO, response DTO, explicit mapping. Field private unless DTO names it. Also checked: `json:"-"` must not appear anywhere under `internal/` outside http adapter (no exemption at all, tests included), and no file named `dto.go` may exist anywhere under `internal/` — check not scoped to feature directory or depth, so `internal/modules/<feature>/http/dto.go` and `internal/platform/dto.go` fail it same as `internal/modules/<feature>/dto.go` does. Exemptions allowlisted by path _with stated reason_ in script — `internal/modules/payment/gateway.go`, external gateway's wire contract not ours — plus `internal/config/` and `internal/platform/` by location.
2. **A feature's `postgres` adapter only names tables it owns.** Ownership read out of `db/OWNERSHIP.md` at run time, so document and check cannot drift. Keywords: `FROM`, `JOIN`, `INSERT INTO`, `UPDATE`, `TRUNCATE`, `COPY`, matched across newlines and through quoted identifiers, over whole `postgres/` subtree. CTE named after real table is own violation, not exemption — else one `WITH orders AS (...)` silences every reference to `orders` in file. Check also validates document itself: duplicate rows, rows for tables no migration creates, and tables no row claims all fail. `dashboard` exempt by name — reporting read-model. Change ownership in `db/OWNERSHIP.md`; no list in script to keep in step.
3. **Nothing outside the wiring layer imports a feature's `postgres`, `http` or
   `redis` package.** Features and shared infrastructure alike; only `internal/bootstrap/` and `internal/transport/` may wire adapters together.

Two more rules are machine-checked, but by `make lint` rather than
`make check-boundaries` — which means `make ci` catches them and
`check-boundaries` does not:

4. **No stdlib `log`, anywhere.** `depguard` denies `pkg: log$` across
   `$all`. There is no `main.go` carve-out: `Run` and `run` report their own
   failures, so `main` needs no logger of its own and holds only the exit
   code.
5. **No `slog.Any`, anywhere.** `forbidigo` denies the identifier. Every
   attribute names its type. An error is
   `slog.String("error", err.Error())` — byte-identical output, because
   slog's JSONHandler already special-cases `error` by calling `Error()`.
   A recovered panic is `slog.String("panic", fmt.Sprint(rec))`, since
   `recover()` returns `any`.

Read "What it does not catch" section of `db/OWNERSHIP.md` before trusting green run. Short: table names must be string literals (`pgx.CopyFrom` included), `_test.go` files skipped on purpose, `dashboard` exempt wholesale, only `internal/modules/<feature>/postgres/` scanned, ownership per table so column coupling invisible, and prose in production string literal can produce loud false positive.

### Conventions — not checked, so they need you

6. **A feature never imports another feature.** Declare interface _the consumer_ needs in consumer's own package (`internal/modules/order/ports.go` declares what `order` needs from inventory), and let `internal/bootstrap/` supply adapter. Often other module's service satisfies interface direct and no adapter written — `promotion.Service` already satisfies `payment.CouponReleaser`, and `notification.Service` already satisfies `jobs.Processor`. No shared ports package, and adding one would defeat point. _(Rule 3 catches crudest violation — importing sibling's adapter — but importing sibling's root package not caught.)_
7. **Services take `database.TxRunner`, never `*pgxpool.Pool`.** Service needs atomicity, not DB handle. `TxRunner` declared once in `internal/platform/database` not per consumer — one deliberate exception to rule 6's consumer-declaration pattern, because features already import `platform/database`. Service that opens no transaction takes no runner at all.
8. **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope is four features: `order`, `payment`, `product`, `cart`. `promotion` and `dashboard` stay on `int64` for stated reasons — `ARCHITECTURE.md` §10 and `ARCHITECTURE-LIMITATIONS.md`. `Money` carries no `json` tag and implements no `sql.Scanner`: each adapter maps it explicit, because wire shapes genuinely differ per endpoint. No float constructor and no `Div`.
9. **A service runs no SQL and holds no pool.** Every read and write goes through feature's repository interface; `postgres` adapter owns pool and reaches it with `database.DB(ctx, pool)`, which returns context's transaction if there is one. Service composes several repository calls into one unit of work via its `TxRunner`, and transaction propagates to every repository — own and other features' — through `ctx`.
10. **Order status changes only through `order.Service.Apply`.** Every guarded transition is named `order.Transition` value in `internal/modules/order/transition.go` (`PaidTransition`, `RefundTransition`, `CancelledTransition`, …). Other features depend on _intent_ methods on their own port interface (`payment.OrderUpdater.MarkPaid`, `shipping.OrderUpdater.MarkShipped`), and `internal/bootstrap/` adapter maps each intent to its transition. Never write ad-hoc from/to status list at call site.
11. **Inventory reversal goes through `inventory.Service.Restore(ctx, items,
prior StockState)`.** Inventory decides whether that means releasing reservation or restocking deducted goods; callers supply order's prior state, never mechanics.
12. **Background job workers use `platform/jobs`.** Feature draining queue implements `jobs.Queue[T]` (`Claim` + `Prune`) on its repository and `jobs.Processor[T]` (`Process`) on its service, plus optional `jobs.Sweeper` for per-tick housekeeping. Binary builds `jobs.Runner[T]`. Never hand-roll ticker/lease/poll loop — runner owns polling, leased compare-and-set claim, bounded concurrency, per-job timeouts and pruning.
13. **Repository reads use `pgx.CollectRows`**, never hand-rolled `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and build keyset predicates with `database.KeysetCursor()`.
14. **Handlers use the shared helpers.** Decode and validate with `response.Bind[T](w, r, h.validator)`; read caller with `middleware.RequireUser(w, r)`; return errors through `response.HandleErr`. Do not hand-roll decode/validate or auth-context blocks.
15. **New config invariants go in `Config.validate()`**
    (`internal/config/config.go`), so misconfiguration aborts boot instead of surfacing later as runtime error. Do not guard per use site.
16. **Request-scoped attributes are named once, at the edge.**
    `logger.WithAttrs(ctx, ...)` stashes them and `logger.ContextHandler`
    merges them into every record below, so no function grows a parameter
    to carry `request_id`. Four edges do this: `middleware.RequestID`
    (`request_id`), `middleware.Auth` (`user_id`), `jobs.Runner.Start`
    (`runner`), and each queue-draining `Process` (`job_id`).
17. **An attribute may only be named at an edge that owns exactly one
    value.** `order_id` and `payment_id` stay written at the call site
    because `order.Service` loops over batches of orders — one context
    cannot hold fifty. Naming an attribute at two points on the same path
    emits the key twice; slog does not deduplicate.

## Code style

- Go 1.26. stdlib `net/http` `ServeMux` — do not add third-party router.
- `encoding/json` for JSON. `log/slog` for logging. `go-playground/validator/v10` for validation. `godotenv` + `kelseyhightower/envconfig` for config.
- Errors: sentinels in `internal/apperror`. Wrap with `fmt.Errorf("%w: ...", apperror.ErrBadRequest)` to add context.
- Packages are short singular nouns (`user`, `product`, `cart`).
- `gofmt -s`, enforced by `make fmt` and golangci-lint. Import groups: stdlib, blank line, third-party, blank line, local (`github.com/residwi/go-api-project-template/...`).
- Comments explain _why_, not _how_. Write one where reader would otherwise read code as mistake. Two patterns to avoid: comment restating what code plainly does, and comment repeating function's doc comment at call site — reason belongs on declaration, never on consumers. When unsure, leave it out.
- Prefer duplication over abstraction that does not quite fit.
- Commit messages: conventional-commit prefixes in use on this branch (`refactor(cart): …`, `docs(db): …`, `test(e2e): …`). Match surrounding history.

## Testing

- `testing` + `stretchr/testify`. `require` when test cannot continue without value, `assert` for soft checks.
- **Docker is required.** No build tags, no short mode. `internal/testhelper` starts two long-lived containers by fixed name (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to whichever already exists. Remove them with `make test-clean`.
- **SQL semantics stay in adapter's own test.** Recursive CTEs, keyset pagination, unique constraints — anything only DB can prove — belong in `internal/modules/<feature>/postgres/repository_test.go` (or `redis/` adapter's own `cache_test.go`) against real container. Anything mock can express — service's reaction to value, error branch — belongs in `service_test.go` instead, and saga spanning tables no single feature owns goes to `test/e2e/` (below). No feature root starts own container any more. `go test ./...` runs package binaries concurrently; collapsing per-package tests into one `test/integration` package would make them sequential. `ARCHITECTURE.md` decision 11 rejects that directory explicit.
- **`test/e2e/` is for sagas no single feature can own** — checkout, payment, refund, fulfilment failure, admin flows — driven through real `apihttp.NewRouter`, real Postgres, and mock gateway on `httptest.Server`.
- **Claim a slot when you add a test package.** `MustStartPostgres(dbName)` drops and recreates that database `WITH (FORCE)`, so two packages sharing name tear each other down mid-run. `MustStartRedis(dbIndex)` takes index from hand-maintained registry comment in `internal/testhelper/testhelper.go`; indices 0, 1, 2, 3, 5, and 6 are taken, 4 is free. Nothing enforces either claim — collision compiles, passes review, fails as flake in unrelated package. Update registry comment in same commit.
- **`t.Parallel()` buys nothing in a package that owns a database or a Redis
  index**, because everything in that package shares one connection and `ResetDB` TRUNCATEs every table in it. Those packages excluded from `paralleltest` wholesale in `.golangci.yml` -- per package, never per file, because parallel sibling gets its rows deleted mid-assertion even when that sibling never calls reset itself. Nothing given up: `go test` already runs packages concurrently and each owns own database. Have each subtest seed own data instead.
- **Everywhere else `t.Parallel()` is mandatory**, and `paralleltest` enforces it on both test function and every `t.Run` closure. If you add test package claiming database or Redis slot, add it to that exclusion list in same commit.
- **Order a test file so the tests come first.** Package-level `var`s and `TestMain` at top, then every `func TestXxx`, then stub types with their own methods grouped under them, then plain helpers last. `internal/platform/jobs/runner_test.go` is the shape. Someone opening file came for scenarios, not fakes that serve them. `funcorder` only orders methods against their struct, so nothing lints the rest — on you.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest, descriptive name, own setup. Break large scenarios up; no monolithic tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
result)` on full struct or slice. For JSONB round-trips use `assert.JSONEq` — Postgres normalises whitespace.
- **Test behaviour, not wiring.** Verify returned value, error, or side effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` **in-package**, as `mocks_test.go` beside interface they mock. A `_test.go` file never enters its package's importable `GoFiles`, so mock is private to that package and cannot cycle back — which lets service test be `package <feature>`, and keeps every `Mock*` name out of feature's exported API. Privacy cuts both ways: any _other_ package needing same mock gets own generated copy, which is why each interface carries `configs:` list in `.mockery.yml` naming every package needing its mock — count varies by interface, asserted by `make check-boundaries` — destinations depend on every module having `http/` adapter and every mocked interface sitting at module root, and mockery silent when either stops holding. `internal/bootstrap` receives `MockProductRepository` / `MockInventoryRepository` under `structname:` — two interfaces both named `Repository` would otherwise collide in one package. Run `make mocks`; never hand-edit generated file. Use expecter API (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`), never `repo.On("GetByID", ...)`.
- **Keep tests fast.** Use `bcrypt.MinCost` for password hashes in tests (`DefaultCost` costs ~250ms per hash) and group tests exercising real `Register` path. Use `testing/synctest` for ticker- and timeout-driven code — `internal/platform/jobs/runner_test.go` does. Note `synctest` cannot wrap `pgxpool` acquire, so test holding real pool must shrink intervals and timeouts instead. Give intentionally-broken clients short timeouts (`MaxRetries: 0`, `DialTimeout: 200 * time.Millisecond`) so error paths fail in milliseconds not seconds.

## Security

- Secrets come from env vars or gitignored `.env`. Never commit real secrets. `.env.example` lists every supported variable.
- JWT auth with configurable expiry; bcrypt password hashes; RBAC via admin middleware.
- Middleware in `internal/transport/http/middleware/`: panic recovery, request-ID injection, structured request logging, CORS, rate limiting, auth, admin.
- Field exposure controlled by DTO omission, not by `json:"-"`. Fourteen `json:"-"` tags used to be load-bearing security controls (`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`) where deleting two characters published password hash. Rule 1 exists for that reason: adding field to response now means naming it in DTO deliberate.

## Guardrails

- Never hand-edit generated `mocks_test.go` — regenerate with `make mocks`.
- Never commit `.env`, secrets or API keys.
- Run `make check-boundaries`, `make vet` and `make test` before calling change complete. `make all` does all three plus lint and build.
- Do not add third-party router.
- Do not suppress lint or vet findings with `//nolint` without justification comment on same line — see `NewRouter`'s for expected form.
- Do not make subpackage tree uniform, and do not add pass-through adapter package to fill slot.
- Backward compatibility explicitly **not** a goal here. API shapes may change where better design demands — but say so when they do.
- When adding feature: create `internal/modules/<feature>/` with own `model.go` / `service.go` / `repository.go`, put SQL in `internal/modules/<feature>/postgres/` and handlers in `internal/modules/<feature>/http/`, add row per owned table to `db/OWNERSHIP.md`, register routes in `internal/transport/http/router.go`, and put any cross-feature adapter in `internal/bootstrap/`. Then run `make check-boundaries` — new feature with `postgres` adapter and no ownership row fails it by design.

## Further reading

- `README.md` — endpoint reference and quick start. Its "Project Structure" section agrees with this file; both rewritten against real tree. Its environment table is **curated subset** — 11 variables absent, including whole Redis pool group. `.env.example` is exhaustive list; verified against `internal/config/config.go`'s `envconfig` tags.
- `ARCHITECTURE.md`, `ARCHITECTURE-LIMITATIONS.md`, `db/OWNERSHIP.md` — as above.
- `db/migrations/` — goose SQL migrations.
- `.env.example`, `.mockery.yml`, `.golangci.yml`.
