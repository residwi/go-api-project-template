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
mocks/                    generated (mockery v3), one subdir per source package
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
| the other 11 | `postgres/ http/` |

`notification` has no `worker/` package because `notification.Service` satisfies
`jobs.Processor` directly. That absence is the lesson — `ARCHITECTURE.md`
decision 4 — not an omission to fix. There are 13 packages named `postgres` and
14 feature packages named `http`, which is why
`internal/transport/http/router.go` needs 27 aliased adapter imports.

Inside `http/`, the file split is by **handler role**, not by endpoint. The
unqualified name is the default handler; `admin_` and `webhook_` are the
qualified exceptions. Every feature holds a subset of exactly these eight names
and nothing else:

| File | Package | Holds |
| --- | --- | --- |
| `routes.go` | `http` | `RouteDeps` and `RegisterRoutes` only — no DTOs, no logic |
| `handler.go` | `http` | the default (public or authed) handler, its DTOs and mappers |
| `handler_test.go` | `http_test` | its route-level tests, driven through a mux |
| `admin_handler.go` | `http` | the admin handler, where routes split by caller role |
| `admin_handler_test.go` | `http_test` | its route-level tests |
| `webhook_handler.go` | `http` | `payment` only — the gateway callback |
| `webhook_handler_test.go` | `http_test` | its route-level tests |
| `internal_test.go` | `http` | unit tests that reach unexported mappers and handlers |

Counted across `internal/modules/*/http/`: `routes.go` ×14, `internal_test.go`
×14, `handler.go` and `handler_test.go` ×13, `admin_handler.go` and
`admin_handler_test.go` ×8, `webhook_handler.go` and `webhook_handler_test.go`
×1. `payment` is the one feature with no `handler.go` — it has no public role.

**`internal_test.go` is not optional.** It is `package http`, and it holds the
leak tests that call unexported mappers (`toProductResponse`, …) directly. Those
are what stop a domain field reaching an unauthenticated response body, and an
external `package http_test` file cannot reach them — Go forbids one file being
both packages, so the second file is a language constraint, not a style choice.
Put a new test where its access requires, then name the file for that.

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
make test-coverage     # ./internal/... ./mocks/... ./test/... -> coverage.out + coverage.html
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
  `./internal/... ./mocks/... ./test/...`.** A new top-level test directory is
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
3. **Nothing outside the wiring layer imports a feature's `postgres` or `http`
   package.** Features and shared infrastructure alike; only
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
- **Integration tests stay colocated** with the code they test
  (`internal/modules/<feature>/postgres/repository_test.go`, and
  `internal/modules/<feature>/*_integration_test.go`). `go test ./...` runs package
  binaries concurrently; collapsing them into one `test/integration` package
  would make them sequential. `ARCHITECTURE.md` decision 11 rejects that
  directory explicitly.
- **`test/e2e/` is for sagas no single feature can own** — checkout, payment,
  refund, fulfilment failure, admin flows — driven through the real
  `apihttp.NewRouter`, a real Postgres, and the mock gateway on an
  `httptest.Server`.
- **Claim a slot when you add a test package.** `MustStartPostgres(dbName)` drops
  and recreates that database `WITH (FORCE)`, so two packages sharing a name tear
  each other down mid-run. `MustStartRedis(dbIndex)` takes an index from the
  hand-maintained registry comment in `internal/testhelper/testhelper.go`;
  indices 0–5 are taken. Nothing enforces either claim — a collision compiles,
  passes review, and fails as a flake in an unrelated package. Update the
  registry comment in the same commit.
- **`t.Parallel()` buys nothing inside a package**, because subtests share one
  database. Have each subtest seed its own data instead.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest,
  a descriptive name, its own setup. Break large scenarios up; no monolithic
  tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
  result)` on the full struct or slice. For JSONB round-trips use
  `assert.JSONEq` — Postgres normalises the whitespace.
- **Test behaviour, not wiring.** Verify a returned value, an error, or a side
  effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` into `mocks/`. Run
  `make mocks`; never hand-edit. Use the expecter API
  (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`), never
  `repo.On("GetByID", ...)`.
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

- Never hand-edit `mocks/` — regenerate with `make mocks`.
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
