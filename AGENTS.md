# AGENTS.md

Orientation for agents and humans working in this repository. It describes the
tree as it actually is, the commands that actually exist, and — most usefully —
which rules are **machine-checked** and which are only conventions.

Three documents carry the reasoning; this one does not duplicate them:

- **`ARCHITECTURE.md`** — the twelve decisions that shaped this codebase and the
  thirteen things it deliberately does not do, each with its cost.
- **`ARCHITECTURE-LIMITATIONS.md`** — what those decisions make hard or
  impossible, and what you would have to build to get past each one. Read this
  before proposing a feature that crosses a module boundary.
- **`db/OWNERSHIP.md`** — which module owns which table, parsed at run time by
  `make check-boundaries`, plus what that check cannot see.

If this file ever disagrees with the code, the code wins — say so and fix the
file.

## What this is

A Go 1.26 ecommerce API template. REST endpoints under `/api` for auth, users,
categories, products, inventory, cart, orders, payments, shipping, reviews,
promotions, wishlists, notifications and an admin dashboard, plus a separate
worker process that drains payment and notification job queues. PostgreSQL via
`pgx/v5`, Redis via `go-redis/v9`, routing on stdlib `net/http` `ServeMux` — no
third-party router.

The structure is the product. It is a template others copy, so a boundary the
compiler or CI can enforce is preferred over one a code review has to.

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
`scripts/check-boundaries.sh` derives the feature list structurally, by reading
the directory names under `internal/modules/`, so adding a feature is enough to
enrol it in the boundary checks; there is no denylist to remember to update.
Being infrastructure exempts a directory from checks 2 and 3's *ownership*
questions, not from check 3 itself: only the wiring layer — `bootstrap` and
`transport`, the script's `WIRING_DIRS` — may import a feature's adapter, so
`internal/platform/` importing `internal/modules/product/postgres` still fails.

### Inside a feature

A feature holds its domain types, its service, its repository *interface*, and
the ports it needs from other features. Adapters are subpackages.

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

Every feature has `model.go`, `service.go` and `repository.go` except `auth`,
which has no storage of its own. There is **no** `handler.go` or `routes.go` at
a feature root — those live in `internal/modules/<feature>/http/`. A `dto.go`
belongs nowhere at all: check 1c refuses that filename **anywhere** under
`internal/`, `http/` included. Wire types live in the handler file that
serialises them.

Ports are usually in `ports.go`, but two features name the file after the module
they depend on instead: `internal/modules/category/product.go` declares `ProductCounter`, and
`internal/modules/product/inventory.go` declares `InventoryReader` and `InventoryRegistrar`.
Either is fine. The rule is about *who declares the interface* (the consumer),
not the filename.

**The subpackage tree is deliberately non-uniform. Do not tidy it into
uniformity.** A feature has a subpackage only where adaptation is needed:

| Feature | Subpackages |
| --- | --- |
| `payment` | `postgres/ http/ stripe/ midtrans/ mock/ worker/` |
| `auth` | `http/` only — no storage; it asks `user` via `auth.UserProvider` |
| `dashboard` | `postgres/ http/` — but owns no table (see the reporting carve-out) |
| `user` | `postgres/ http/ redis/` — the only feature with a second backing store |
| the other 10 | `postgres/ http/` |

`notification` has no `worker/` package because `notification.Service` satisfies
`jobs.Processor` directly. That absence is the lesson — `ARCHITECTURE.md`
decision 4 — not an omission to fix. `user/redis/` is the positive case of the
same rule: a subpackage exists where a feature has that kind of backing store,
and `user` is the only feature caching, so `ls internal/modules/user/` still
tells the truth about which features do. A feature declares one port per
store — `repository.go` for Postgres, `cache.go` for a cache — and gets one
adapter subpackage per port: `user.Repository` pairs with `postgres/`,
`user.StatusCache` with `redis/`. That adapter requires Redis 8.0 or later; it
is built on `HSETEX`, which sets a hash's fields and their expiry in a single
atomic command, and that command does not exist on earlier Redis. There are 13
packages named `postgres`, 14 feature packages named `http`, and one named
`redis`, which is why `internal/transport/http/router.go` needs 28 aliased
adapter imports.

Inside `http/`, the file split is by **handler role**, not by endpoint. The
unqualified name is the default handler; `admin_` and `webhook_` are the
qualified exceptions. Every feature holds a subset of exactly these eight names
and nothing else:

| File | Package | Holds |
| --- | --- | --- |
| `routes.go` | `http` | `RouteDeps` and `RegisterRoutes` only — no DTOs, no logic |
| `handler.go` | `http` | the default (public or authed) handler, its DTOs and mappers |
| `handler_test.go` | `http` | its route-level tests, driven through a mux, plus the leak tests for its unexported mappers |
| `admin_handler.go` | `http` | the admin handler, where routes split by caller role |
| `admin_handler_test.go` | `http` | its route-level tests plus the leak tests for its unexported mappers |
| `webhook_handler.go` | `http` | `payment` only — the gateway callback |
| `webhook_handler_test.go` | `http` | its route-level tests |

Counted across `internal/modules/*/http/`: `routes.go` ×14, `handler.go` and
`handler_test.go` ×11, `admin_handler.go` and `admin_handler_test.go` ×10,
`webhook_handler.go` and `webhook_handler_test.go` ×1.

Three features have **no** `handler.go`, and that is the naming rule working
rather than an omission: `payment`, `dashboard` and `inventory` register every
route on the admin group, so their only handler is an `adminHandler`. If a
feature's `http/` has no `handler.go`, it has no non-admin surface — which is a
fact worth being able to read off `ls`.

**Tests live in the package they test, except where an import cycle forbids
it.** `handler_test.go`, `admin_handler_test.go` and `webhook_handler_test.go`
are `package http`, holding both the route-level tests driven through a mux
and the leak tests that call unexported mappers (`toProductResponse`, …)
directly — the tests that stop a domain field reaching an unauthenticated
response body. Being in-package permits white-box testing without preventing
black-box testing, so one file now does both; that dissolves the old
constraint that split them, which is why the separate leak-test file per
feature is gone — its contents moved beside the implementation file declaring
what they test.

Two carve-outs remain, both cycles rather than preferences, and together
they are the whole exception: 10 external test files, no more. The service
tests are no longer among them: mocks generate in-package, so a mock no
longer imports the package it mocks, and every feature-root `_test.go` file
— `service_test.go` included — is `package <feature>`.
`test/e2e` (9 files, `package e2e_test`) imports concrete adapters —
`internal/modules/*/postgres`, `internal/bootstrap`,
`internal/transport/http` — across every module the saga touches; no single
feature package can own that without becoming a dependent of its siblings,
which `make check-boundaries` forbids. `internal/testhelper/txrunner_test.go`
(1 file, `package testhelper_test`) asserts `database.TxRunner` is satisfied
from outside `testhelper`, which cannot import `platform/database` itself —
`platform/database`'s own in-package tests import `testhelper` for
`MustStartPostgres`, so the dependency can only run the other way in an
external file. Go's own standard library draws the per-file line the same
way: `net/http` ships 19 in-package test files (`package http`) beside 18
external `package http_test` files in the same directory, choosing per file
by the access the test needs, not one package-wide policy. Put a new test
where its access requires, then name the file for that.

## Commands

All verified against the `Makefile`; `make help` lists them with descriptions.

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

Two things about those last two are worth knowing:

- **`make ci` does not run `check-boundaries`; `make all` does.** If you rely on
  one command before calling work finished, use `make all`, or run
  `make check-boundaries` explicitly.
- **`make test` runs `./...` while `make test-coverage` globs
  `./internal/... ./test/...`.** A new top-level test directory is
  picked up by the first and silently skipped by the second.

Database commands need the goose CLI (`make migrate-install`; the Makefile
expects it at `$(go env GOPATH)/bin/goose`). They build `DATABASE_URL` from
`DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE`,
all overridable:

```bash
make migrate-up  migrate-down  migrate-down-all  migrate-status  migrate-version
make migrate-create name=add_something
make db-create  db-drop  seed
make docker-up  docker-dev  docker-down  docker-logs  docker-build  docker-clean
```

## Architectural rules

### Machine-checked

`make check-boundaries` runs `scripts/check-boundaries.sh` and fails the build on
any of these. This is the part worth memorising, because these are the rules you
cannot violate quietly.

1. **No `json` tag outside `internal/modules/<feature>/http/`.** Domain models carry no
   transport concerns; every endpoint owns its request DTO, response DTO and
   explicit mapping. A field is private unless a DTO names it. Also checked:
   `json:"-"` must not appear anywhere under `internal/` outside an http adapter
   (no exemption at all, including tests), and no file named `dto.go` may exist
   anywhere under `internal/` — the check is not scoped to a feature directory or
   to a depth, so `internal/modules/<feature>/http/dto.go` and
   `internal/platform/dto.go` fail it just as `internal/modules/<feature>/dto.go`
   does. Exemptions are allowlisted by path *with a stated reason* in the
   script — `internal/modules/payment/gateway.go`, which is the external
   gateway's wire contract rather than ours — plus `internal/config/` and
   `internal/platform/` by location.
2. **A feature's `postgres` adapter only names tables it owns.** Ownership is
   read out of `db/OWNERSHIP.md` at run time, so the document and the check
   cannot drift. Keywords: `FROM`, `JOIN`, `INSERT INTO`, `UPDATE`, `TRUNCATE`,
   `COPY`, matched across newlines and through quoted identifiers, over the
   whole `postgres/` subtree. A CTE named after a real table is its own
   violation rather than an exemption — otherwise one `WITH orders AS (...)`
   silences every reference to `orders` in the file. The check also validates
   the document itself: duplicate rows, rows for tables no migration creates,
   and tables no row claims all fail. `dashboard` is exempt by name — it is a
   reporting read-model. Change ownership in `db/OWNERSHIP.md`; there is no list
   in the script to keep in step.
3. **Nothing outside the wiring layer imports a feature's `postgres`, `http` or
   `redis` package.** Features and shared infrastructure alike; only
   `internal/bootstrap/` and `internal/transport/` may wire adapters together.

Read the "What it does not catch" section of `db/OWNERSHIP.md` before trusting a
green run. In short: table names must be string literals (`pgx.CopyFrom`
included), `_test.go` files are skipped on purpose, `dashboard` is exempt
wholesale, only `internal/modules/<feature>/postgres/` is scanned, ownership is per
table so column coupling is invisible, and prose in a production string literal
can produce a loud false positive.

### Conventions — not checked, so they need you

4. **A feature never imports another feature.** Declare the interface *the
   consumer* needs in the consumer's own package (`internal/modules/order/ports.go` declares what
   `order` needs from inventory), and let `internal/bootstrap/` supply the
   adapter. Often the other module's service satisfies the interface directly and
   no adapter is written — `promotion.Service` already satisfies
   `payment.CouponReleaser`, and `notification.Service` already satisfies
   `jobs.Processor`. There is no shared ports package, and adding one would
   defeat the point. *(Rule 3 catches the crudest violation — importing a
   sibling's adapter — but importing a sibling's root package is not caught.)*
5. **Services take `database.TxRunner`, never `*pgxpool.Pool`.** A service needs
   atomicity, not a database handle. `TxRunner` is declared once in
   `internal/platform/database` rather than per consumer — the one deliberate
   exception to rule 4's consumer-declaration pattern, because features already
   import `platform/database`. A service that opens no transaction takes no
   runner at all.
6. **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope
   is four features: `order`, `payment`, `product`, `cart`. `promotion` and
   `dashboard` stay on `int64` for stated reasons — `ARCHITECTURE.md` §10 and
   `ARCHITECTURE-LIMITATIONS.md`. `Money` carries no `json` tag and implements no
   `sql.Scanner`: each adapter maps it explicitly, because the wire shapes
   genuinely differ per endpoint. There is no float constructor and no `Div`.
7. **A service runs no SQL and holds no pool.** Every read and write goes through
   the feature's repository interface; the `postgres` adapter owns the pool and
   reaches it with `database.DB(ctx, pool)`, which returns the context's
   transaction if there is one. A service composes several repository calls into
   one unit of work via its `TxRunner`, and the transaction propagates to every
   repository — its own and other features' — through the `ctx`.
8. **Order status changes only through `order.Service.Apply`.** Every guarded
   transition is a named `order.Transition` value in `internal/modules/order/transition.go`
   (`PaidTransition`, `RefundTransition`, `CancelledTransition`, …). Other
   features depend on *intent* methods on their own port interface
   (`payment.OrderUpdater.MarkPaid`, `shipping.OrderUpdater.MarkShipped`), and
   the `internal/bootstrap/` adapter maps each intent to its transition. Never
   write an ad-hoc from/to status list at a call site.
9. **Inventory reversal goes through `inventory.Service.Restore(ctx, items,
   prior StockState)`.** Inventory decides whether that means releasing a
   reservation or restocking deducted goods; callers supply the order's prior
   state, never the mechanics.
10. **Background job workers use `platform/jobs`.** A feature draining a queue
    implements `jobs.Queue[T]` (`Claim` + `Prune`) on its repository and
    `jobs.Processor[T]` (`Process`) on its service, plus optional `jobs.Sweeper`
    for per-tick housekeeping. The binary builds a `jobs.Runner[T]`. Never
    hand-roll a ticker/lease/poll loop — the runner owns polling, the leased
    compare-and-set claim, bounded concurrency, per-job timeouts and pruning.
11. **Repository reads use `pgx.CollectRows`**, never a hand-rolled
    `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and
    build keyset predicates with `database.KeysetCursor()`.
12. **Handlers use the shared helpers.** Decode and validate with
    `response.Bind[T](w, r, h.validator)`; read the caller with
    `middleware.RequireUser(w, r)`; return errors through `response.HandleErr`.
    Do not hand-roll decode/validate or auth-context blocks.
13. **New config invariants go in `Config.validate()`**
    (`internal/config/config.go`), so misconfiguration aborts boot instead of
    surfacing later as a runtime error. Do not guard per use site.

## Code style

- Go 1.26. stdlib `net/http` `ServeMux` — do not add a third-party router.
- `encoding/json` for JSON. `log/slog` for logging. `go-playground/validator/v10`
  for validation. `godotenv` + `kelseyhightower/envconfig` for config.
- Errors: sentinels in `internal/apperror`. Wrap with
  `fmt.Errorf("%w: ...", apperror.ErrBadRequest)` to add context.
- Packages are short singular nouns (`user`, `product`, `cart`).
- `gofmt -s`, enforced by `make fmt` and golangci-lint. Import groups: stdlib,
  blank line, third-party, blank line, local
  (`github.com/residwi/go-api-project-template/...`).
- Comments explain *why*, not *how*. Write one where a reader would otherwise
  read the code as a mistake.
- Prefer duplication over an abstraction that does not quite fit.
- Commit messages: conventional-commit prefixes are in use on this branch
  (`refactor(cart): …`, `docs(db): …`, `test(e2e): …`). Match the surrounding
  history.

## Testing

- `testing` + `stretchr/testify`. `require` when the test cannot continue without
  the value, `assert` for soft checks.
- **Docker is required.** There are no build tags and no short mode.
  `internal/testhelper` starts two long-lived containers by fixed name
  (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to
  whichever already exists. Remove them with `make test-clean`.
- **SQL semantics stay in the adapter's own test.** Recursive CTEs, keyset
  pagination, unique constraints — anything only the database can prove —
  belong in `internal/modules/<feature>/postgres/repository_test.go` (or a
  `redis/` adapter's own `cache_test.go`) against a real container. Anything a
  mock can express — a service's reaction to a value, an error branch —
  belongs in `service_test.go` instead, and a saga spanning tables no single
  feature owns goes to `test/e2e/` (below). No feature root starts its own
  container any more. `go test ./...` runs package binaries concurrently;
  collapsing per-package tests into one `test/integration` package would make
  them sequential. `ARCHITECTURE.md` decision 11 rejects that directory
  explicitly.
- **`test/e2e/` is for sagas no single feature can own** — checkout, payment,
  refund, fulfilment failure, admin flows — driven through the real
  `apihttp.NewRouter`, a real Postgres, and the mock gateway on an
  `httptest.Server`.
- **Claim a slot when you add a test package.** `MustStartPostgres(dbName)` drops
  and recreates that database `WITH (FORCE)`, so two packages sharing a name tear
  each other down mid-run. `MustStartRedis(dbIndex)` takes an index from the
  hand-maintained registry comment in `internal/testhelper/testhelper.go`;
  indices 0, 1, 2, 3, 5, and 6 are taken, and 4 is free. Nothing enforces
  either claim — a collision compiles, passes review, and fails as a flake in
  an unrelated package. Update the registry comment in the same commit.
- **`t.Parallel()` buys nothing in a package that owns a database or a Redis
  index**, because everything in that package shares the one connection and
  `ResetDB` TRUNCATEs every table in it. Those packages are excluded from
  `paralleltest` wholesale in `.golangci.yml` -- per package, never per file,
  because a parallel sibling gets its rows deleted mid-assertion even when that
  sibling never calls a reset itself. Nothing is given up: `go test` already
  runs packages concurrently and each owns its own database. Have each subtest
  seed its own data instead.
- **Everywhere else `t.Parallel()` is mandatory**, and `paralleltest` enforces
  it on both the test function and every `t.Run` closure. If you add a test
  package that claims a database or Redis slot, add it to that exclusion list
  in the same commit.
- **Order a test file so the tests come first.** Package-level `var`s and
  `TestMain` at the top, then every `func TestXxx`, then the stub types with
  their own methods grouped under them, then the plain helpers last.
  `internal/platform/jobs/runner_test.go` is the shape. Someone opening the
  file came for the scenarios, not for the fakes that serve them. `funcorder`
  only orders methods against their struct, so nothing lints the rest of this
  — it is on you.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest,
  a descriptive name, its own setup. Break large scenarios up; no monolithic
  tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
  result)` on the full struct or slice. For JSONB round-trips use
  `assert.JSONEq` — Postgres normalises the whitespace.
- **Test behaviour, not wiring.** Verify a returned value, an error, or a side
  effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` **in-package**, as
  `mocks_test.go` beside the interface they mock. A `_test.go` file never enters
  its package's importable `GoFiles`, so the mock is private to that package and
  cannot cycle back to it — which is what lets a service test be
  `package <feature>`, and keeps every `Mock*` name out of the feature's exported
  API. That privacy cuts both ways: any *other* package needing the same mock
  gets its own generated copy, which is why each interface carries a
  `configs:` list in `.mockery.yml` naming every package that needs its mock —
  the count varies by interface, asserted by `make check-boundaries` — the destinations depend
  on every module having an `http/` adapter and every mocked interface sitting at
  a module root, and mockery is silent when either stops holding. `internal/bootstrap`
  receives `MockProductRepository` / `MockInventoryRepository` under
  `structname:` — two interfaces both named `Repository` would otherwise collide
  in one package. Run `make mocks`; never hand-edit a generated file.
  Use the expecter API (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`),
  never `repo.On("GetByID", ...)`.
- **Keep tests fast.** Use `bcrypt.MinCost` for password hashes in tests
  (`DefaultCost` costs ~250ms per hash) and group the tests that exercise the
  real `Register` path. Use `testing/synctest` for ticker- and timeout-driven
  code — `internal/platform/jobs/runner_test.go` does. Note `synctest` cannot
  wrap a `pgxpool` acquire, so a test holding a real pool must shrink intervals
  and timeouts instead. Give intentionally-broken clients short timeouts
  (`MaxRetries: 0`, `DialTimeout: 200 * time.Millisecond`) so error paths fail in
  milliseconds rather than seconds.

## Security

- Secrets come from env vars or a gitignored `.env`. Never commit real secrets.
  `.env.example` lists every supported variable.
- JWT auth with configurable expiry; bcrypt password hashes; RBAC via the admin
  middleware.
- Middleware in `internal/transport/http/middleware/`: panic recovery, request-ID
  injection, structured request logging, CORS, rate limiting, auth, admin.
- Field exposure is controlled by DTO omission, not by `json:"-"`. Fourteen
  `json:"-"` tags used to be load-bearing security controls
  (`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`) where
  deleting two characters published a password hash. Rule 1 exists for that
  reason: adding a field to a response now means naming it in a DTO
  deliberately.

## Guardrails

- Never hand-edit a generated `mocks_test.go` — regenerate with `make mocks`.
- Never commit `.env`, secrets or API keys.
- Run `make check-boundaries`, `make vet` and `make test` before calling a change
  complete. `make all` does all three plus lint and build.
- Do not add a third-party router.
- Do not suppress lint or vet findings with `//nolint` without a justification
  comment on the same line — see `NewRouter`'s for the expected form.
- Do not make the subpackage tree uniform, and do not add a pass-through adapter
  package to fill a slot.
- Backward compatibility is explicitly **not** a goal here. API shapes may change
  where the better design demands it — but say so when they do.
- When adding a feature: create `internal/modules/<feature>/` with its own
  `model.go` / `service.go` / `repository.go`, put SQL in `internal/modules/<feature>/postgres/`
  and handlers in `internal/modules/<feature>/http/`, add a row per owned table to
  `db/OWNERSHIP.md`, register routes in `internal/transport/http/router.go`, and
  put any cross-feature adapter in `internal/bootstrap/`. Then run
  `make check-boundaries` — a new feature with a `postgres` adapter and no
  ownership row fails it by design.

## Further reading

- `README.md` — endpoint reference and quick start. Its "Project Structure"
  section agrees with this file; both were rewritten against the real tree. Its
  environment table is a **curated subset** — 11 variables are absent, including
  the whole Redis pool group. `.env.example` is the exhaustive list; it is
  verified against `internal/config/config.go`'s `envconfig` tags.
- `ARCHITECTURE.md`, `ARCHITECTURE-LIMITATIONS.md`, `db/OWNERSHIP.md` — as above.
- `db/migrations/` — goose SQL migrations.
- `.env.example`, `.mockery.yml`, `.golangci.yml`.
