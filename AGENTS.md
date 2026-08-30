# AGENTS.md

Orientation for agents and humans in this repo. Describes tree as it actually is, commands that actually exist, and — most useful — which rules **machine-checked** vs which only convention.

Three docs carry the reasoning; this one no duplicate:

- **`ARCHITECTURE.md`** — nineteen decisions that shaped this codebase, fifteen things it deliberately not do, each with cost. Decision 14 is marked **reversed** and kept as history: it is why this tree held 226 packages of vertical slices for a year, and decision 16 records what replaced it.
- **`ARCHITECTURE-LIMITATIONS.md`** — what those decisions make hard or impossible, and what you must build to get past each. Read before proposing feature that crosses module boundary.
- **`db/OWNERSHIP.md`** — which module owns which table, parsed at run time by `make check-boundaries`, plus what that check cannot see.

If this file ever disagree with code, code wins — say so and fix file.

## What this is

Go 1.26 ecommerce API template. REST endpoints under `/api` for auth, users, categories, products, inventory, cart, orders, payments, shipping, reviews, promotions, wishlists, notifications, admin dashboard, plus separate worker process draining payment, notification and order job queues. PostgreSQL via `pgx/v5`, Redis via `go-redis/v9`, routing on stdlib `net/http` `ServeMux` — no third-party router.

Structure is product. Others copy template, so boundary compiler or CI can enforce beats boundary code review must.

## Repository structure

```text
cmd/api/                  API server binary
cmd/worker/               payment + notification + order job worker binary
cmd/mockgateway/          dev-only fake payment gateway binary
  mockserver/             its handlers, importable so internal/server can mount them in-process
internal/
  apperror/               seven cross-module business sentinels
                          (ErrInsufficientStock, ErrCartEmpty, ...), each declared as
                          a wrap of a platform/errs kind; no feature deps
  bootstrap/              the composition root: builds every Service and wires every
                          cross-module port by name-match
  server/                 server.go (Run), router.go (NewRouter, health, routes) and
                          middleware/, which is down to the three files that know a
                          caller identity -- auth.go, admin.go, ratelimit.go.
                          router.go holds every URL in the system -- all 64 of them,
                          in one function
  platform/               generic infrastructure, no feature deps: cache/ config/
                          database/ errs/ jobs/ logger/ paging/ response/ slug/
                          storage/ validator/ web/
  testutil/               shared dockertest harness (Postgres + Redis containers)
  modules/<feature>/      16 directories: the 14 features, plus checkout and money
db/migrations/            goose SQL migrations
db/seeds/data.sql         seed data, applied by `make seed`
db/OWNERSHIP.md           table -> owning module, read by the boundary check
test/e2e/                 cross-module saga tests through the real router
scripts/check-boundaries.sh   the architectural checks
scripts/boundaries_test.go    probes proving each check still matches something
```

**Three different counts live under `internal/modules/`. Say which one you
mean.** Getting this wrong is the single easiest mistake to make in this repo.

- **16 directories** — `ls -1 internal/modules | wc -l`. That is what
  `scripts/check-boundaries.sh` sees: it derives the list structurally from
  directory names, so a new module is enrolled in the checks the day its
  directory exists. No denylist to remember.
- **15 with an `adapter/http`** — `ls -d internal/modules/*/adapter/http | wc -l`.
  Every one but `money`.
- **14 features** — `auth cart category dashboard inventory notification order
  payment product promotion review shipping user wishlist`.

The two directories that are not features:

- **`checkout`** is a bounded context. It owns no table, no `domain/` and no
  store; it orchestrates `order` and `payment` across one business
  transaction, which is what broke the order/payment cycle. All it holds is
  `service.go`, `ports.go` and an `adapter/http` with three routes.
- **`money`** is a shared kernel: the `Money` value object. No `Service`, no
  store, no table, no routes, and it imports no other module. It lives under
  `internal/modules/` because it is a **value object** — a domain type — and
  check 4 makes any module's root package importable, so `cart`, `order`,
  `payment` and `product` name `money.Money` with no exemption anywhere.
  `internal/apperror` stays above the module tree for a narrower reason than it
  once had. It used to hold the whole error vocabulary and be named by
  `platform` and `server` as well, which made it nobody's to own. Those five
  generic kinds are `internal/platform/errs` now, and `internal/server` imports
  `apperror` nowhere at all — every importer today is a module
  (`cart checkout inventory order payment promotion`). What is left is seven
  business sentinels that several modules raise and several others match, and
  no one module has the better claim to them, so they sit above rather than
  inside any of the seven.

Everything else under `internal/` is infrastructure — `apperror bootstrap
platform server testutil`. Being infrastructure exempts a directory from
checks 2 and 3's _ownership_ questions: those loop over `feature_dirs`, that
is `internal/modules/*` only, so `internal/platform` is never in their scan.
It does **not** exempt it from check 4. Only the wiring layer — `bootstrap`
and `server`, the script's `WIRING_DIRS` — may import a module's internals, so
`internal/platform` importing `internal/modules/order/domain` or
`internal/modules/order/adapter/postgres` fails exactly as another module
importing it would. Check 6 runs the other way and is narrower: it loops over
`feature_dirs` only, so it polices whether a *module* imports
`internal/server`, not whether `internal/platform` does.

### Inside a module

**A module is one flat package plus an `adapter/` directory.** There is no
`usecase/` left anywhere — `find internal -type d -name usecase` prints
nothing — no `module.go`, and no `Module` type. `ls
internal/modules/<feature>/` tells the truth for any of the sixteen:

```text
internal/modules/<feature>/
  service.go         one exported Service, plus New. No Deps struct anywhere
                     in the tree -- New takes its dependencies as positional
                     parameters, so a forgotten one is a missing-argument
                     compile error rather than a silently-nil struct field.
                     15 of 16 -- money has none, being a value object
  repository.go      the storage port adapter/postgres satisfies (13)
  ports.go           every cross-module port this module consumes, one file (9)
  contract.go        the struct types another module may name (8)
  config.go          this module's own env vars (5: auth cart notification
                     order payment)
  domain/            aggregate types and rules -- private, and check 4
                     enforces it (14: all but checkout and money)
  job.go             a background job's Kind, payload and Run -- payment
                     (RefundJob), notification (SendJob) and order
                     (ExpireStaleJob) only (3)
  channel.go         the outbound Channel port -- notification only, paired
                     with adapter/channel/log
  service_test.go    mock-driven tests, package <feature>
  mocks_test.go      mockery output, in-package, invisible outside the tests
  adapter/
    postgres/        SQL adapter, where the module has SQL (13)
    http/            handlers plus their wire types (15)
    redis/           user only -- the store behind its StatusCache port
    gateway/         payment only -- the outbound Gateway port and its three
                     real implementations
    channel/         notification only -- the outbound Channel port's log
                     implementation
```

Re-run the numbers rather than trust them:

```bash
ls -1 internal/modules | wc -l                 # 16 directories
ls internal/modules/*/service.go | wc -l       # 15 services
ls internal/modules/*/repository.go | wc -l    # 13 repository ports
ls internal/modules/*/ports.go | wc -l         #  9 ports files
ls internal/modules/*/contract.go | wc -l      #  8 contract files
ls -d internal/modules/*/adapter/http | wc -l  # 15 http adapters
```

No module holds a directory at its root outside `domain/` and `adapter/` any
more — zero (`find internal/modules -mindepth 2 -maxdepth 2 -type d ! -name
domain ! -name adapter` prints nothing). This refactor moved the last two
that did: `payment/gateway/` — the outbound `Gateway` port and its three real
implementations (`stripe/ midtrans/ mock/`), picked once in
`payment/service.go`'s `newGateway` from `Config.Gateway` — moved to
`payment/adapter/gateway/`, and the two job-queue directories that used to
sit beside it, `payment/jobs/postgres/` and `notification/jobs/`, are gone
outright. The job store they each kept — a `JobRepository` port and a
`jobs.Worker` respectively — collapsed into one `internal/platform/jobs/postgres`
store the platform layer owns, not either module. See rule 16.

#### Naming

**One `Service` per module, and its methods carry the verb.** `grep -rn 'func
(s \*Service) Execute' internal/modules` returns nothing. Four rules came out
of the flatten, and they are worth knowing before adding a method:

1. **No `Execute`.** `place.UseCase.Execute` read correctly because the
   package said `place`. One `Service` cannot hold five `Execute` methods, so
   each took the verb its directory used to carry — `order.Service.Place`,
   `cart.Service.Add`, `payment.Service.Refund`.
2. **No module-name stutter.** The receiver already names the module.
   `cart.Get`, not `cart.GetCart`. `payment.Charge`, not
   `payment.InitiatePayment`. `order.RecoverStale`, not
   `order.RecoverStaleProcessing`.
3. **An entity module implies its object; a process module names it.**
   `category.Create` and `order.Get` read correctly because the receiver names
   the entity being acted on, so repeating it would stutter. `checkout` names
   a process — `checkout.Create` would read as "create a checkout", which is
   not what it does — so its methods carry their object: `PlaceOrder`,
   `RetryPayment`, `CancelOrder`. `Place` rather than `Create` for the same
   reason: the operation locks the cart, validates it, reserves inventory,
   reserves a coupon, writes the order, charges and clears the cart. `Create`
   is CRUD vocabulary for a row insert and under-describes all of that.
4. **A port is named for the role it plays**, never for the pattern — rule
   18a.

`GetForUser` beside `Get` is the convention: the `ForUser` suffix marks the
method that performs an ownership check, the plain one does not.
`order.GetForUser` and `shipping.GetForUser` keep it consistent.

#### Ports

**A cross-module port is declared in the consuming module's own `ports.go` —
one file per module, nine files, 16 interfaces** (`grep -h '^type .* interface'
internal/modules/*/ports.go | wc -l`). The consumer names the interface;
the producer never publishes it. `category/ports.go` declares `ProductCounter`
(`CountPublished`, one method). `order/ports.go` declares four and
`payment/ports.go` four. A module that reaches nothing outside itself has no
`ports.go` at all — `dashboard inventory money notification promotion user
wishlist`, seven of the sixteen.

Two mechanisms satisfy a port without an adapter, and `internal/bootstrap` is
the one place either is used:

- **Name-match.** The producer's own value already has a method named what the
  consumer's port asks for. `promotion.Service` satisfies both
  `order.CouponReserver` (`Reserve` + `Release`) and `payment.CouponReleaser`
  (`Release` alone) — two differently-shaped interfaces, one producer value,
  no adapter for either. `*order.Service` satisfies four port
  fields across four consumers — `payment.Orders`, `checkout.Orders`,
  `shipping.Orders` and `review.PurchaseVerifier` — one port apiece now that
  every port collapses to one per producer.
- **A `contract.go` type**, when what crosses is a struct rather than a scalar
  or something a producer already satisfies by name.

**One port per producer, not one per capability.** A module declares one
interface for each other module it consumes, holding every method it needs
from that producer. `order` declares `Cart`, `Inventory`, `CouponReserver`
and `Notifications` — four ports for four producers, where it used to
declare eight for the same four. The cost is legibility: a consumer's port
now lists methods a given call path does not use, so reading `order.Cart` no
longer tells you that `Place` needs exactly `Lock`, `Snapshot` and `Clear`.

That has a price the sliced shape did not pay. Two ports bound to two
*different* slice values, so the compiler checked each `Deps` field against the
value it was handed; one flat `Service` satisfying both means it cannot.
Nothing misbehaves today — wherever one `Service` satisfies several of a
consumer's ports, every one of those fields is wired to that same value, so
there is no wrong value to assign. `ARCHITECTURE-LIMITATIONS.md` prices the
guarantee that went away and says when the absence starts to bite.

A `dto.go` belongs nowhere at all: check 1c refuses that filename **anywhere**
under `internal/`. Wire types live in the module's own `adapter/http`, in the
file that serialises them.

#### `contract.go`

**Eight of the sixteen have one** — `auth cart inventory notification order
payment product user`. It holds the struct types another module's port names
in a signature: `user.Profile` and `user.Credentials`, `cart.Snapshot`,
`order.Snapshot`, `product.Info`, `inventory.Availability` and
`inventory.StockState`, `auth.ClaimsView`, `payment.ChargeRequest`,
`notification.NewNotification` (the payload `order.Notifications.Create`
passes across). A module earns one only when a *struct* has to cross a port —
not a scalar, and not something a producer's `Service` already satisfies by
name. The eight without one never pass a struct across, which is why they
have nothing to show.

`contract.go` is a file in the module's root package now, not the separate
`contract/` package it was, and that traded away a guarantee. Check 7
(`check_contract_leaf`) used to prove that the package a consumer imported for
a published type pulled in nothing but stdlib, `uuid` and `money`. A root
package imports its own `domain/` by design, so the check had nothing left to
be true of and was retired. `ARCHITECTURE-LIMITATIONS.md` records the cost.

#### `adapter/`

**The subpackage tree stays non-uniform — do not tidy it into uniformity.** A
module gets an `adapter/postgres` only if it has SQL, an `adapter/http` only
if it has a route, a `ports.go` only if it reaches outside itself. `auth` has
no store anywhere in the module: it asks `user` for everything through one
port (`auth.UserDirectory`) and keeps nothing of its own. `checkout` has
neither a store nor a `domain/`. `user` is the one module with two backing
stores and the only `adapter/redis`: `user.Repository` pairs with
`adapter/postgres`, and `user.StatusCache` — declared in its own `cache.go`,
not in `repository.go` — pairs with `adapter/redis`. One adapter package per
port. That Redis adapter needs Redis 8.0 or later; it is built on `HSETEX`,
which earlier Redis does not have. Read `ls` for the module in front of you. A
table enumerating every module's shape belongs nowhere in this file: three
such paragraphs have already gone stale by doing exactly that.

Inside `adapter/http`, files split by **handler role**. Unqualified
`handler.go` is the default (public or authed) handler; `admin_handler.go` and
`webhook_handler.go` are the qualified exceptions. One handler may carry
several routes: `checkout`'s three — place, retry, cancel — are one `Handler`
in one file, because they are the same caller role acting on the same order.
`routes.go` never appears here, or anywhere under `internal/modules/` — every
URL lives in `internal/server/router.go`, see rule 10. Seven modules carry an
`admin_handler.go` beside a `handler.go` because they split their own routes by
caller role (`category order product promotion review shipping user`);
`payment`'s only public route is the gateway callback,
so it has `admin_handler.go` and `webhook_handler.go` and no `handler.go` at
all.

Wire types split the same way, into up to four files: `request.go` and
`response.go` hold the public/authed shapes, `admin_request.go` and
`admin_response.go` hold the admin-only ones. **A type shared between roles
goes in the unqualified file, not in both.** `order` and `shipping` are the
two dual-role modules with no `admin_response.go` at all: `admin_handler.go`
calls the same `toOrderResponse` / `toShipmentResponse` the public handler
does, because an admin sees the same shape a caller with ownership does. The
other four dual-role modules — `category`, `product`, `promotion`, `user` —
each declare their own `admin_response.go`, because an admin response
genuinely carries fields the public one omits (`toAdminUserResponse`'s nine
fields against `toUserResponse`'s five is the example decision 9 and
`ARCHITECTURE-LIMITATIONS.md` both use). `review`'s admin route needs no
response type at all — `DELETE` returns `response.NoContent`, so nothing
crosses the wire either way.

**The route methods on those handlers are exported**, and that is not
cosmetic: `internal/server/router.go` is a different package in a different
tree, and it can only mount `orderHandler.List`, `userAdminHandler.Get`,
`checkoutHandler.Place` if it can name them.

The port a handler takes is declared locally, in the handler's own package,
and is named for the role it needs — 23 interfaces across the 15 `adapter/http`
packages, none of them called `UseCase`
(`grep -hoE '^type [A-Za-z]+ interface' internal/modules/*/adapter/http/*.go`).
`CartManager`, `ProductReader`, `WebhookProcessor`, `Reporter`. Eight packages
hold two ports, which is what role naming buys: naming both `UseCase` would
redeclare an identifier. See rule 18a.

#### Tests

**Tests live in the package they test, except where an import cycle forbids
it.** `handler_test.go` and its qualified siblings are `package http`, holding
route-level tests driven through a mux *and* tests that call unexported
mappers (`toUserResponse`, …) directly — the ones that stop a domain field
reaching an unauthenticated response body. In-package testing permits
white-box tests without preventing black-box ones, so one file does both.

**A handler test writes its own URL, and that URL is not the production one.**
Every test file under `internal/modules/*/adapter/http/` builds its own
`middleware.NewRouteGroup` and registers the handler on a prefix it picks;
several write `/api/v1/...`, which production has never served. None imports
`internal/server`. So a handler test proves the handler, never the URL nor the
group it lands on. **`internal/server/routes_snapshot_test.go`,
`internal/server/router_test.go` and `test/e2e/` are the only things that
drive the real `NewRouter`**, and only the first enumerates the whole table:
`TestRouteSnapshot` reads `internal/server/testdata/routes.golden` — 64 lines
of `method<TAB>path<TAB>group` — and probes every one. What it still cannot
see is in `ARCHITECTURE-LIMITATIONS.md`.

Five carve-outs put a test outside the package it tests — two forced by an
import cycle, two by preference, one because the thing under test is not Go —
and together they are the whole exception: **17 external test files**
(`grep -rl '^package .*_test$' --include='*_test.go' . | wc -l`).

- `test/e2e` (12 files, `package e2e_test`) imports `internal/bootstrap` and
  `internal/server` to drive the real router end to end, plus `auth`, `cart`
  and `payment`'s root packages for their `LoadConfig`, and `payment/domain`
  for a type to assert on. No module's adapter: `bootstrap.New` is the one
  place that wires one up. A saga crossing five modules cannot live inside any
  one of them without making that module depend on its siblings, which `make
  check-boundaries` forbids.
- `internal/testutil/txrunner_test.go` (`package testutil_test`) asserts
  `database.TxRunner` is satisfied from outside `testutil`, which cannot
  import `platform/database` itself — `platform/database`'s own in-package
  tests import `testutil` for `MustStartPostgres`, so the dependency can only
  run the other way, in an external file.
- `internal/bootstrap/app_test.go` and `internal/testutil/testutil_test.go`
  are the preference carve-out. `app_test.go`'s own doc comment says it
  exercises `bootstrap.New` from outside "the way every real caller does";
  neither file touches an unexported identifier or an import that would cycle,
  so both could have run in-package and chose the outside view instead.
- `scripts/boundaries_test.go` (`package scripts_test`) shells out to
  `scripts/check-boundaries.sh`, plants a probe file in a real module and
  asserts the script reports it. There is no Go package for it to be inside.
- `internal/apperror/apperror_test.go` (`package apperror_test`) is a second
  preference carve-out. It asserts that each of the seven business sentinels
  unwraps to its `errs` kind, which is the view a consumer has, and
  `internal/apperror` declares nothing unexported for it to reach.
  `internal/modules/auth/errors_test.go` makes the same two assertions about
  the two auth sentinels and is `package auth`, because `modules/auth` already
  has in-package tests and a second package clause in that directory would buy
  nothing.

A module's `service_test.go` is never among them: mocks generate in-package,
so a mock never imports the package it mocks, and every one is `package
<feature>`. Go's own standard library draws the line per file the same way —
`net/http` ships in-package `package http` test files beside external `package
http_test` ones in the same directory, choosing per file by what the test
needs to reach. Put a new test where its access requires, then name the file
for that.

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
make test-one NAME=X   # go test -run X across ./... with .env loaded
make test-coverage     # ./internal/... ./test/... -> coverage.out + coverage.html
make test-clean        # remove the shared postgres + redis test containers
make mocks             # regenerate every mocks_test.go from .mockery.yml

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

`scripts/check-boundaries.sh` registers **six** checks, and every rule below
names the function that enforces it so the two cannot drift apart by name
alone. **They are numbered 1, 2, 3, 4, 6 and 8. The gaps are deliberate.**
Checks 5 and 7 were retired, and renumbering the survivors would falsify every
by-number citation here, in `ARCHITECTURE.md` and in `db/OWNERSHIP.md` at
once. A gap also states something a closed list would hide: a check used to be
here, and why it is not.

1. **`check_wire_tags`: a `json` tag lives only in a module's own
   `adapter/http`.** The exempt location is
   `internal/modules/<feature>/adapter/http/*.go` and nothing shorter:
   `internal/modules/<feature>/http/` is deliberately **not** exempt, because
   that path held the feature route tables until they moved, and a json tag
   reappearing there would mean a DTO had drifted out of the module that owns
   it. `internal/server/` is not exempt either — the router wires modules and
   declares no wire type of its own. That one exempt arm carries every module:
   remove it and check 1 reports **294** tags in fifteen adapters at once.
   Also checked: `json:"-"` must not appear anywhere under `internal/` outside
   an `adapter/http` (no exemption at all, tests included — there are **zero**
   in the tree today), and no file named `dto.go` may exist anywhere under
   `internal/`. One path is allowlisted by name _with a stated reason_ in the
   script — `internal/modules/payment/adapter/gateway/gateway.go`, the
   external gateway's wire contract rather than ours — plus
   `internal/platform/` by location. That location arm covers two things at
   once: `internal/platform/config/`, whose tags are `envconfig` and not
   `json` but whose exemption matters so that adding one is not mistaken for a
   domain leak, and `internal/platform/response/response.go`, the shared
   envelope every handler writes through. The envelope used to need a name of
   its own, at `internal/server/response/response.go`; that entry was deleted
   when the package moved under `internal/platform`, where it is exempt by
   location like everything else there.
2. **`check_ownership_doc`: `db/OWNERSHIP.md` itself has no duplicate row, no
   row for a table no migration creates, and no table with no owning row.**
   Parsed out of the document at run time, between the BEGIN/END OWNERSHIP
   TABLE markers — the document and the check read the same list, so they
   cannot drift.
3. **`check_table_ownership`: a module's SQL only names tables it owns.**
   Scans **every non-test `.go` file under the module**, not only its
   `adapter/postgres` — a query in `service.go` is caught the same as one in
   the adapter. Keywords: `FROM`, `JOIN`, `INSERT INTO`, `UPDATE`, `TRUNCATE`,
   `COPY`, matched across newlines and through quoted identifiers. A module is
   skipped only when it has no `postgres/` directory anywhere under it (a
   legitimate no-storage module — `auth`, `checkout`, `money`). Only a match
   against a table actually listed in `db/OWNERSHIP.md` is reported: the
   identifier after a keyword must be a real table name, not merely a word
   absent from the scanning module's own list, or a `slog` call like `"failed
   to update payment status"` would report a violation on every run. A CTE
   named after a real table is its own violation, not an exemption — one
   `WITH orders AS (...)` would otherwise silence every genuine reference to
   `orders` in the file. `dashboard` is exempt by name: reporting read-model.
   Change ownership in `db/OWNERSHIP.md`; there is no list in the script to
   keep in step.
4. **`check_cross_module_imports`: a module may import another module's root
   package — that is its published surface — and nothing deeper.** `domain/`
   and every adapter (`postgres`, `http`, `redis`, `jobs`) stay private to the
   module that owns them. This is the reverse of the rule that stood while the
   tree was sliced, where the root package was off-limits and `contract/` was
   the only door. Exempt as importers: `internal/bootstrap/` and
   `internal/server/` (the script's `WIRING_DIRS`) — empty that list and the
   check reports **30** imports in the wiring layer alone. Same-module imports
   are unrestricted, which is the target shape: a module's `adapter/postgres`
   importing its own root package for `var _ order.Repository` is exactly
   right. **One per-importer exemption exists: `checkout` alone may import a
   module's `domain/`**, because `order.Service.Place`'s signature is written in
   `orderdomain.NewOrder` and `*orderdomain.Order` and `order/contract.go`
   publishes neither. It is keyed on the importer and not on the target, so
   `checkout` importing `payment/domain` passes just as cleanly; `order/domain`
   is simply the only one it needs. Removing that exemption reports **7** violations, not
   zero. It is a real weakening of the rule for one module of sixteen, and
   `ARCHITECTURE-LIMITATIONS.md` names it as its own limitation.
5. **RETIRED — `check_sibling_slice_imports`.** It refused a slice importing a
   sibling slice inside its own module, a rule check 4 structurally could not
   see. With no `usecase/` tree left anywhere it walked nothing and could only
   pass, so it was deleted along with its probe. Nothing replaces it: the
   coupling it prevented needed two peer packages inside one module, and there
   is one `Service` per module now. The number is left vacant on purpose.
6. **`check_transport_direction`: a module may not import `internal/server`,
   except its own `adapter/http`.** That is the one exempt location, and it
   still carries real weight, though much less than it used to: removing the
   arm reports **20** imports across **nine** modules in one run — `cart
   checkout notification order promotion review shipping user wishlist`. It
   used to report 85 across all fifteen, and the drop is the measure of what
   moved: `response.Bind` is `internal/platform/response` now, so the only
   reason left to import `internal/server` is `middleware.RequireUser`,
   `SetUserContext` and `UserContext` (rule 18) — caller identity, which is
   exactly what stayed. The other six `adapter/http` packages — `auth
   category dashboard inventory payment product` — serve routes that never
   name a caller and import `internal/server` nowhere at all. The check
   catches a `Service` returning a transport type, and a module that would
   register its own routes — either would make every binary constructing the
   module link HTTP, including the worker, which serves nothing. A module that
   needs to describe something the transport also describes puts the type in
   its own `contract.go` and lets middleware import the module root instead:
   `user.AccountStatus` and `auth.ClaimsView` are what that looks like.
7. **RETIRED — `check_contract_leaf`.** It held every `contract/` package to
   stdlib, `uuid` and `internal/modules/money` only, so that importing a
   module's published types could never drag its `domain/` along. Zero
   `contract/` directories remain: those types are declared in `contract.go`
   in each module's root package, which imports `domain/` by design, so the
   rule had nothing left to be true of. What it guaranteed is genuinely gone
   — see `ARCHITECTURE-LIMITATIONS.md`.
8. **`check_platform_leaf`: nothing under `internal/platform` may import a
   local package outside `internal/platform`.** The point is portability:
   `cp -r internal/platform` into a fresh module has to compile with no edits,
   and one import reaching upward ends that silently, in a diff that reads as
   obviously fine. The test is written the other way round from every other
   import check here — it matches **every** import of this repository's own
   code and then subtracts what is allowed, rather than naming the trees that
   are forbidden. That is deliberate: the first version named
   `internal/modules`, `internal/server` and `internal/apperror`, and so said
   nothing about `internal/bootstrap` or `cmd/mockgateway/mockserver`, either
   of which would end the property just as completely. A closed list also only
   covers a tree added later on the day someone remembers to extend it.
   **`internal/testutil` is the one exemption**, named in the script with its
   reason: three platform test packages import it for the shared dockertest
   harness (`platform/database`, `platform/cache`, `platform/jobs/postgres`).
   It is a hole rather than a tidy carve-out, and it is why the copy property
   holds for `go build` on a copied `internal/platform` and **not** for `go
   test` — `internal/testutil` does not live under `internal/platform` and so
   does not travel with it. Closing it means moving `internal/testutil` down
   there; until someone does, it is a stated limitation, recorded here, in the
   script and in `ARCHITECTURE-LIMITATIONS.md`.

`scripts/boundaries_test.go` is what keeps a path-keyed check from dying
quietly. It plants a probe file in a real module — a json tag, a `dto.go`, a
foreign `FROM orders`, a `domain/` import, an adapter import, an
`internal/server` import, an upward import from `internal/platform` — and
asserts the script reports each one. It also
probes from the other side, asserting that a module's root-package import, the
wiring layer's adapter import, `checkout`'s `order/domain` import and an
intra-platform import all stay
clean, so an exemption that has stopped matching anything fails a test instead
of printing `Boundaries OK`. Run it with `go test ./scripts/`.

Two more rules are machine-checked, but by `make lint` rather than
`make check-boundaries` — which means `make ci` catches them and
`check-boundaries` does not. They share the number 9 because 8 is now a
boundary check and the conventions below start at 10, and moving either would
falsify a citation somewhere:

9a. **No stdlib `log`, anywhere.** `depguard` denies `pkg: log$` across
   `$all`. There is no `main.go` carve-out: `Run` and `run` report their own
   failures, so `main` needs no logger of its own and holds only the exit
   code.
9b. **No `slog.Any`, anywhere.** `forbidigo` denies the identifier. Every
   attribute names its type. An error is
   `slog.String("error", err.Error())` — byte-identical output, because
   slog's JSONHandler already special-cases `error` by calling `Error()`.
   A recovered panic is `slog.String("panic", fmt.Sprint(rec))`, since
   `recover()` returns `any`.

Read "What it does not catch" in `db/OWNERSHIP.md` before trusting a green
run. Short: table names must be string literals (`pgx.CopyFrom` included),
`_test.go` files skipped on purpose, `dashboard` exempt wholesale, ownership
per table so column coupling invisible, and prose in a production string
literal can produce a loud false positive. Two more, wider than any of those:
every check walks `internal/` only, so `cmd/` and `test/` are outside all five
— and both reach `payment/domain` today, unreported. And none of the five is a
compiler; they are all greps.

### Conventions — not checked, so they need you

10. **A module imports another module's root package and nothing deeper.**
    The root package *is* the published surface — `order.Snapshot`,
    `user.Profile`, `promotion.Service`'s methods. `domain/` and every adapter
    stay private. This reverses what the sliced tree enforced, where the root
    package was off-limits and `contract/` was the one door, and the cost is
    named plainly: **`payment` can now see `order.Place`.** Nothing stops a
    module calling a method on a sibling `Service` that no port of its own
    declares, and no check can tell the difference — check 4 sees a legal
    root-package import either way. What used to be a compile error is now a
    convention. Declare the interface *the consumer* needs in the consumer's
    own `ports.go` and wire it in `internal/bootstrap`; do not reach for a
    sibling's method directly because the import already compiles.
    `ARCHITECTURE-LIMITATIONS.md` opens with this trade.

    **The arrow runs the other way for URLs: the transport imports modules,
    and a module names no URL.** Every route lives in
    `internal/server/router.go` — inside `NewRouter` itself, 64 routes in
    fifteen labelled blocks, mounted on the route groups that same function
    builds. A module supplies a handler with exported route methods; the
    transport decides the verb, the path, and which `middleware.RouteGroup` it
    lands on. Check 6 enforces the direction; `ARCHITECTURE.md` decision 15 is
    why, and its cost.

    Two mechanisms satisfy a port without an adapter:
    - **Name-match.** The producer's own value already has a method named what
      the consumer's port asks for. `promotion.Service` satisfies both
      `order.CouponReserver` (`Reserve` + `Release`) and `payment.CouponReleaser`
      (`Release` alone) directly — two differently-shaped interfaces, one
      producer value, no adapter for either. `*order.Service` satisfies
      four port fields across four consumers — `payment.Orders`,
      `checkout.Orders`, `shipping.Orders` and `review.PurchaseVerifier` — and
      `internal/bootstrap/app.go` hands the same value to all four. A consumer
      now declares one port per producer rather than one per capability, so
      each of the four holds exactly one interface for `order`, not several.
    - **A `contract.go` type**, when what crosses is a struct rather than a
      scalar or an interface a producer already satisfies. The consumer's port
      still names the type it needs (`checkout.Orders.Snapshot` returns
      `order.Snapshot`); `contract.go` supplies only the shape, never the
      interface.

    No shared ports package, and adding one would defeat the point.
11. **Services take `database.TxRunner`, never `*pgxpool.Pool`.** Service needs atomicity, not DB handle. `TxRunner` declared once in `internal/platform/database` not per consumer — one deliberate exception to rule 10's consumer-declaration pattern, because modules already import `platform/database`. A module that opens no transaction takes no runner at all: six `Service`s hold one (`cart notification order payment promotion shipping`) — `notification` needs one to make writing its row and enqueueing its `SendJob` one unit of work.
12. **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope is four features: `order`, `payment`, `product`, `cart`. `promotion` and `dashboard` stay on `int64` for stated reasons — `ARCHITECTURE.md` §10 and `ARCHITECTURE-LIMITATIONS.md`. `Money` carries no `json` tag and implements no `sql.Scanner`: each adapter maps it explicit, because wire shapes genuinely differ per endpoint. No float constructor and no `Div`.
13. **A `Service` runs no SQL and holds no pool.** Every read and write goes through the module's own `Repository` interface; `adapter/postgres` owns the pool and reaches it with `database.PrimaryDB(ctx, db)` or `database.ReplicaDB(ctx, db)`, where `db` is a `database.DB{Primary, Replica *pgxpool.Pool}` value — both functions return the context's transaction if there is one, and `ReplicaDB` falls back to `Primary` when no replica is configured. Five adapters call `ReplicaDB` for their read-only methods (`order`, `product`, `promotion`, `user`, `dashboard`); every other read and every write goes through `PrimaryDB`. A `Service` composes several repository calls into one unit of work via its `TxRunner`, and the transaction propagates to every repository it touches — its own and other modules' — through `ctx`. **`internal/bootstrap` is what constructs the adapters**: `bootstrap.New` threads one `database.DB` value through every `xxxpg.New(db)` call and hands the result to each module's `New` as a positional argument, so the pool never reaches a `Service` and there is no `module.go` left to hide the wiring in. No `Service` takes a raw `*pgxpool.Pool` any more — `payment` and `notification` used to, to build their own job-queue adapter, but that adapter is now `internal/platform/jobs/postgres`, built once in `bootstrap` as `jobspg.New(db)` and handed to both as the same `jobs.Store` value.
14. **Order status changes only through `order.Service.Apply`.** Every guarded transition is a named `domain.Transition` value in `internal/modules/order/domain/transition.go` (`PaidTransition`, `RefundTransition`, `CancelledTransition`, …). Other modules depend on _intent_ methods on their own port interface (`payment.Orders.MarkPaid`; `shipping.Orders.MarkShipped` and `shipping.Orders.MarkDelivered`, one port holding both intents), and every caller wires to `order.Service`, since `Apply` and the eight `Mark*` methods that forward to it live there and nowhere else. Never write an ad-hoc from/to status list at a call site.
15. **Inventory reversal goes through `inventory.Service.Restore`.**
    `Restore(ctx, items map[uuid.UUID]int, prior StockState) error` decides
    whether that means releasing a reservation or restocking deducted goods;
    callers supply order's prior state, never the mechanics. `StockState` is
    declared in `inventory/contract.go`, so `order` and `payment` name the
    type by importing `inventory`'s root package — never
    `inventory/adapter/postgres`. That import is no longer the leaf it was:
    while `StockState` sat in `inventory/contract/`, check 7 guaranteed the
    package a consumer imported for it pulled in nothing but stdlib, `uuid`
    and `money`. The root package imports `inventory/domain` and declares
    `Service` and `Repository`, and no check holds it to a leaf-import rule.
    `ARCHITECTURE-LIMITATIONS.md` records what that costs.
16. **Background jobs share one platform-owned queue, one table, one store.** There is no per-module job queue any more: `payment/jobs.go`, `payment/adapter/jobs.Dispatcher` and `notification/jobs.Worker` are gone, and `payment_jobs` / `notification_jobs` are gone with them. In their place, `internal/platform/jobs` (`jobs.go`) declares one `Job` interface — `Kind() string` and `Run(ctx context.Context) error` — one `Record` (a `job_queue` row: queue, kind, JSON payload, dedup key, group key, status, attempts, lease), and one `Store` interface (`Insert`, `Claim`, `Complete`, `Retry`, `Bury`, `Cancel`, `CancelByGroupKey`, `Prune`) that `internal/platform/jobs/postgres` implements against that one table. A module declares its own job type in its own `job.go`, at module root, with an exported payload and an unexported dependency: `payment.RefundJob{PaymentID, OrderID; svc *Service}`, `notification.SendJob{NotificationID; svc *Service}` and `order.ExpireStaleJob{At, Every; svc *Service}` are the three that exist today. `jobs.Enqueue[T Job](ctx, e Enqueuer, job T, keys Keys, opts...)` marshals the job to JSON and inserts a `Record`; `queueOf` derives the queue name from the kind's prefix before its first dot, so `"payment.refund"` claims from the `payment` queue, `"notification.send"` from `notification`, and `"order.expire-stale"` from `order`. `internal/bootstrap` builds one `jobs.Registry`, calls `jobs.Register` once per job type to map a `Kind()` to its handler, and hands the same `*jobs.Registry` and shared `jobs.Store` to `cmd/worker`. An unregistered kind is discarded via the `jobs.ErrDiscard` sentinel rather than retried. `cmd/worker` starts one `jobs.Runner` per queue name, all three handed the same `*jobs.Registry` directly with no wrapper type. The order/payment recovery sweep that used to run as a `jobs.Sweeper` type-asserted onto the payment runner is gone: `order.ExpireStaleJob.Run` now recovers stale payment-processing orders and expires stale awaiting-payment orders itself, and enqueues its own successor *before* doing either, so a run that gets buried after exhausting its retries never breaks the recurrence -- the dedup key names the target timestamp (`"order.expire-stale:" + at.UTC().Format(time.RFC3339)`) rather than a fixed string, so scheduling the successor while the current occurrence is still `processing` never collides with `ux_job_queue_active`. `bootstrap` registers it by name-match (`jobs.Register(reg, order.NewExpireStaleJob(ordMod))`) and `cmd/worker` seeds the first occurrence once at boot via `order.Service.ScheduleExpireStale`, using the order runner's own poll interval as the recurrence period. Never hand-roll a ticker/lease/poll loop — the runner owns polling, leased compare-and-set claim, bounded concurrency, per-job timeouts and pruning.
17. **Repository reads use `pgx.CollectRows`**, never a hand-rolled `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and build keyset predicates with `database.KeysetCursor()`.
18. **Handlers use the shared helpers.** Decode and validate with `response.Bind[T](w, r, h.validator)`; read caller with `middleware.RequireUser(w, r)`; return errors through `response.HandleErr`. Do not hand-roll decode/validate or auth-context blocks.
18a. **An `adapter/http` port is named for the role it plays, never for the
    pattern.** `CartManager`, `ProductReader`, `WebhookProcessor`,
    `PromotionApplier`, `Reporter` — 23 ports across the 15 `adapter/http`
    packages, no exceptions
    (`grep -hoE '^type [A-Za-z]+ interface' internal/modules/*/adapter/http/*.go`).
    Never `UseCase`, and never `Service` either: the port is what the *handler*
    asks for, which is a subset of what the module's `Service` offers, and
    reusing the producer's name would say otherwise. Eight packages hold two
    ports, because they split routes by caller role — `user` has
    `ProfileManager` + `UserManager`, `product` has `ProductReader` +
    `ProductManager`. Role naming is what lets them coexist; one shared name
    would redeclare an identifier. The other seven hold one port each, and a
    single port may carry several routes: `checkout.Checkout` covers place,
    retry and cancel, since splitting one caller role three ways bought
    nothing.

    **The `Handler` field holding that port is `service`, and so is the
    constructor parameter that sets it**: `h.service.PlaceOrder(...)`, in all
    23. The constructors are `NewHandler` (14), `NewAdminHandler` (8) and
    `NewWebhookHandler` (1). The old `usecase` field name and bare `New`
    constructor retired with the slices. A field is private and there is only ever one, so it is named for
    the layer, not the role — never `cmd`, `reader` or `svc`.
19. **New config invariants go in the owning type's own loader.** Infra-level
    invariants go in `Infra.validate()` (`internal/platform/config/config.go`);
    module-owned invariants are checked inline inside that module's own
    `LoadConfig` (`auth.LoadConfig`, `cart.LoadConfig`,
    `notification.LoadConfig`, `order.LoadConfig`, `payment.LoadConfig`),
    since each module loads its own env vars now and
    there is no longer one central `Config.validate()` for every invariant to
    share. Either way, misconfiguration aborts boot instead of surfacing later
    as a runtime error. Do not guard per use site.
20. **Request-scoped attributes are named once, at the edge.**
    `logger.WithAttrs(ctx, ...)` stashes them and `logger.ContextHandler`
    merges them into every record below, so no function grows a parameter
    to carry `request_id`. Four edges do this: `middleware.RequestID`
    (`request_id`), `middleware.Auth` (`user_id`), `jobs.Runner.Start`
    (`runner`), and `Runner.processOne` (`job_id`) — one method on the shared
    runner now, not one per queue's own `Process`, since both queues drain
    through the same `Runner` type.
21. **An attribute may only be named at an edge that owns exactly one
    value.** `order_id` and `payment_id` stay written at the call site
    because a command loops over batches of orders — one context
    cannot hold fifty. Naming an attribute at two points on the same path
    emits the key twice; slog does not deduplicate.
22. **Exported `type`, `var` and `const` declarations come before unexported
    ones in a file.** Two exemptions, and only two: a
    `var _ Iface = (*T)(nil)` compile assertion stays adjacent to the type it
    asserts about — `internal/modules/order/adapter/postgres/repository.go`'s
    `var _ order.Repository = (*Repository)(nil)` sits immediately above
    `type Repository`, not at the bottom with every other exported
    declaration, because moving it away separates a two-line proof from the
    thing it proves — and a context-key type stays beside the functions that
    read it rather than at the end of the file:
    `internal/platform/database/transaction.go`'s `txCtxKey` sits between
    `DBTX` and `WithTx`, its only reader; `logger/context.go`'s `ctxKey`
    beside `WithAttrs`; `middleware/auth.go`'s `userCtxKey` beside
    `SetUserContext`/`GetUserContext`. Do not chase this past what it covers:
    the rule is about `type`/`var`/`const` declarations, not every
    identifier, and it is not violated by an unexported type that carries an
    *exported* method moving down with that method — `funcorder`'s
    `struct-method` check already requires a struct's methods to follow its
    own declaration, so seven unexported types with an exported method
    (`poolTxRunner.Run`, `recoverWriter.Write`/`WriteHeader`,
    `statusRecorder.WriteHeader`, and others — `grep -rhoE
    '^func \(\w+ \*?[a-z][A-Za-z]*\) [A-Z][A-Za-z]*'
    --include='*.go' internal cmd | sort -u` finds them) sit below the
    exported types they serve, and that is correct, not a gap to fix.
    `.golangci.yml`'s `funcorder.function: true` is meant to enforce ordering
    on free functions too, but a golangci-lint v2.12.2 bug means it never
    actually fires there — confirmed against the standalone `funcorder`
    binary, which does flag it — so this rule is a convention to read for,
    not yet something `make lint` catches on its own.

## Code style

- Go 1.26. stdlib `net/http` `ServeMux` — do not add third-party router.
- `encoding/json` for JSON. `log/slog` for logging. `go-playground/validator/v10` for validation. `godotenv` + `kelseyhightower/envconfig` for config.
- Errors: five generic kinds in `internal/platform/errs` (`ErrNotFound`,
  `ErrConflict`, `ErrBadRequest`, `ErrUnauthorized`, `ErrForbidden`), seven
  cross-module business sentinels in `internal/apperror`. Wrap with
  `fmt.Errorf("%w: ...", errs.ErrBadRequest)` to add context.
- **A business sentinel is declared as a wrap of a generic kind, never with a
  bare `errors.New`.** `response.HandleErr` is five rows long and matches
  nothing but the five `errs` kinds, so a sentinel that unwraps to none of
  them is a 500 with no way for a caller to tell it from a database outage.
  Nothing catches that: `internal/apperror/apperror_test.go`'s table is
  hand-written, so an eighth sentinel added without an eighth row fails no
  test. The declaration is the only place this can be got right —
  `apperror.ErrOrderCharging` is a 409 because it wraps `errs.ErrConflict`,
  not because any transport file says so.
- **`errors.Is(err, errs.ErrConflict)` now matches every business sentinel
  that wraps it**, where `errors.Is(err, apperror.ErrConflict)` used to match
  only the generic one. Nothing in the tree relies on the old narrowness —
  all twelve `errors.Is(..., errs.*)` call sites were traced when the split
  landed — but a new `errors.Is` against a generic kind is now a broader
  question than it looks. Match the business sentinel when you mean the
  business case.
- Packages are short singular nouns (`user`, `product`, `cart`).
- `gofmt -s`, enforced by `make fmt` and golangci-lint. Import groups: stdlib, blank line, third-party, blank line, local (`github.com/residwi/go-api-project-template/...`).
- **No comments in Go source, except directives and two named exceptions.** This is narrower than "explain why, not how": a comment strip removed every prose comment in the tree, and nothing has been allowed back in since except what earns its place by name, not by category. Directives always stay — every `//nolint:` (with its justification on the same line; see Guardrails) and every `//go:` — because those are instructions to the toolchain, not prose for a reader. Two specific comments survive because deleting them lets a future edit reopen a bug this refactor already paid to close: `internal/platform/jobs/runner.go`'s doc comment above `leaseSafetyDivisor` (without it, `lease - lease/5` reads as arithmetic to tidy away, and tidying it reopens a window where two workers run the same job), and `internal/modules/checkout/service.go`'s comment above `if created && order.Total.Amount > 0` (without it, the `created &&` guard reads as redundant, and deleting it reintroduces the double-charge `6c8bc0f` fixed). Do not add a third by analogy — a comment earns a place here by naming a specific regression it prevents, not by resembling one of these two.
- Prefer duplication over abstraction that does not quite fit.
- Commit messages: conventional-commit prefixes in use on this branch (`refactor(cart): …`, `docs(db): …`, `test(e2e): …`). Match surrounding history.

## Git history

History stays linear. No `Merge branch …` commit — it says nothing about the code and makes `git log` a graph to decode instead of a list to read.

- **Rebase feature branch onto `main` before integrating**, then merge fast-forward:
  `git switch main && git merge --ff-only <branch>`. If fast-forward refuses, branch is behind — rebase again, do not fall back to a merge commit.
- **`git pull --rebase`, never plain `git pull`.** Set it once so it cannot be forgotten: `git config pull.rebase true`.
- Rebase early and often while branch is alive. Cost of rebasing scale with how far the two lines have drifted, so daily is cheap and monthly is not.

**Rebase only works on history that is already linear.** It replay commits in one sequence, so any merge it cross disappear and the two sides interleave into an order that never existed — a commit then land on code it was never written against, and conflict cascade for the rest of the replay. Three merges predate this rule; do not rebase across them.

## Testing

- `testing` + `stretchr/testify`. `require` when test cannot continue without value, `assert` for soft checks.
- **Docker is required.** No build tags, no short mode. `internal/testutil` starts two long-lived containers by fixed name (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to whichever already exists. Remove them with `make test-clean`. **Every package binary races for the same container name**, and `getOrCreateContainer` decides who wins: the loser polls until the winner's container reports running with a bound port, and a container still not running after a grace period (`transientGrace`) is treated as abandoned and purged, so a container wedged mid-start does not wedge every other test binary with it.
- **SQL semantics stay in the adapter's own test.** Recursive CTEs, keyset pagination, unique constraints — anything only the database can prove — belong in `internal/modules/<feature>/adapter/postgres/repository_test.go` (or `adapter/redis/cache_test.go`) against a real container. Anything a mock can express — a `Service`'s reaction to a value, an error branch — belongs in that module's `service_test.go` instead, and a saga spanning tables no single module owns goes to `test/e2e/`. No module starts its own container. `go test ./...` runs package binaries concurrently; collapsing per-package tests into one `test/integration` package would make them sequential. `ARCHITECTURE.md` decision 11 rejects that directory explicitly.
- **`test/e2e/` is for sagas no single module can own** — checkout, payment, refund, fulfilment failure, admin flows — driven through the real `server.NewRouter`, real Postgres, and the mock gateway on an `httptest.Server`.
- **Postgres databases are per module, created once under an advisory lock, and never dropped; Redis indices are still a slot you claim.** `MustStartPostgres(dbName)` creates and migrates `dbName` the first time any caller asks for it — the lock covers the migration too, so a second caller that finds the database already there finds it at the latest schema — and every later caller, same test binary or a different one, just connects. It holds one raw `pgx.Conn`, not a pool, across the exists-check and the `CREATE DATABASE`: every package binary dials the admin database at once when the suite starts, so a single dial routinely comes back "connection reset by peer" and needs the retry `MustStartPostgres` already wraps it in, where a pool would acquire lazily and `Ping` would only prove one connection out of many had worked. **24 packages call `testutil.MustStart*` today** (`grep -rl 'testutil.MustStart' --include='*_test.go' . | xargs -n1 dirname | sort -u | wc -l`), and the mapping is one database per module: `test_cart`, `test_order`, `test_payment`, and so on (`grep -rn 'MustStartPostgres(' --include='*_test.go' internal/modules` is the live mapping). `notification` and `payment` no longer share a database name with a second test package — their old `jobs/postgres` packages are gone along with the per-module job queues, so every Postgres-claiming package today owns its database name outright. **`ResetDB` is safe only for a package that owns its database outright.** Four callers today: `internal/bootstrap/app_test.go` (`test_bootstrap`), `internal/server/router_test.go` (`test_server`), `test/e2e/testmain_test.go` (`test_e2e`) and `internal/platform/jobs/postgres/store_test.go` (`test_platform_jobs`) — the new package added when the job queue moved to `platform`. `ResetDB` also never truncates `goose_db_version`: migrations run unconditionally on every `MustStartPostgres` call, so truncating the version table would make the next caller see zero applied versions and fail against a schema that is already fully migrated. `ResetDB` takes a `*pgxpool.Pool`, not a package name, so nothing stops a fifth caller inside a module adding it — check before copying a `setup` helper that calls it. `MustStartRedis(dbIndex)` takes an index the caller picks by hand — the registry used to be tracked in a comment above that function in `internal/testutil/testutil.go`, but the comment strip removed it, so the list below and `grep -rn 'MustStartRedis(' --include='*_test.go' .` are what's left to check against. Indices 0, 1, 3, 5 and 6 are claimed (`platform/cache`, `server/middleware`, `server`, `test/e2e`, `modules/user/adapter/redis`); 2 and 4 are free. Nothing enforces a claim — a collision compiles, passes review, and fails as a flake in an unrelated package — so update this list in the same commit that takes an index.
- **`t.Parallel()` buys nothing in a package that owns a database or a Redis
  index**, because everything in that package shares one connection and `ResetDB` TRUNCATEs every table in it. Those packages excluded from `paralleltest` wholesale in `.golangci.yml` -- per package, never per file, because parallel sibling gets its rows deleted mid-assertion even when that sibling never calls reset itself. That exclusion's `path:` regex is **mixed**: nine of its eleven alternatives are anchored to the repo root (`^db/(migrations|seeds)/`, `^test/e2e/`, `^internal/testutil/`, and six more), two are not (`/postgres/`, `/redis/`). Only an anchored alternative dies silently when the directory it names moves, and the anchored nine carry most of the weight -- 75 of the 105 files this pattern matches are reached by an anchored alternative and nothing else -- so check those against `git ls-files` after any structural change. `/postgres/` is the single widest alternative in the file at 28 files and needs no such check: it follows `adapter/postgres` wherever that goes, and it picked up `internal/platform/jobs/postgres/` — the one new database-owning package this refactor added — with no edit to the exclusion at all. Nothing given up: `go test` already runs packages concurrently and each owns own database. Have each subtest seed own data instead.
- **Everywhere else `t.Parallel()` is mandatory**, and `paralleltest` enforces it on both test function and every `t.Run` closure. If you add test package claiming database or Redis slot, add it to that exclusion list in same commit.
- **Order a test file so the tests come first.** Package-level `var`s and `TestMain` at top, then every `func TestXxx`, then stub types with their own methods grouped under them, then plain helpers last. `internal/platform/jobs/runner_test.go` is the shape. Someone opening file came for scenarios, not fakes that serve them. `funcorder` only orders methods against their struct, so nothing lints the rest — on you.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest, descriptive name, own setup. Break large scenarios up; no monolithic tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
result)` on full struct or slice. For JSONB round-trips use `assert.JSONEq` — Postgres normalises whitespace.
- **Test behaviour, not wiring.** Verify returned value, error, or side effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` **in-package**, as `mocks_test.go` beside the interface they mock. A `_test.go` file never enters its package's importable `GoFiles`, so a mock is private to that package and cannot cycle back — which lets a module's own `service_test.go` stay `package <feature>` and its `handler_test.go` stay `package http`, each mocking only the interfaces declared in its own package, and keeps every `Mock*` name out of the module's exported API. 36 `mocks_test.go` files exist (`find internal -name mocks_test.go | wc -l`). `.mockery.yml` is one recursive rule rooted at `internal/`, `all: true`, no per-interface `configs:` list — every interface gets exactly one mock, in its own package. This is why an `adapter/http` handler declares its own narrow port locally rather than importing one from the module's root package: mockery cannot write a private mock into a package that does not declare the interface, so an interface consumed across a package boundary has to be declared on the consumer's side either way. Run `make mocks`; never hand-edit the generated file. Use the expecter API (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`), never `repo.On("GetByID", ...)`.
- **Keep tests fast.** Use `bcrypt.MinCost` for password hashes in tests (`DefaultCost` costs ~250ms per hash) and group tests exercising real `Register` path. Use `testing/synctest` for ticker- and timeout-driven code — `internal/platform/jobs/runner_test.go` does. Note `synctest` cannot wrap `pgxpool` acquire, so test holding real pool must shrink intervals and timeouts instead. Give intentionally-broken clients short timeouts (`MaxRetries: 0`, `DialTimeout: 200 * time.Millisecond`) so error paths fail in milliseconds not seconds.

## Security

- Secrets come from env vars or gitignored `.env`. Never commit real secrets. `.env.example` lists every supported variable.
- JWT auth with configurable expiry; bcrypt password hashes; RBAC via admin middleware.
- Middleware lives in two packages. `internal/platform/web` holds panic
  recovery, request-ID injection, structured request logging and CORS, plus
  `Middleware`, `Chain` and `RouteGroup`. `internal/server/middleware` holds
  auth, admin and rate limiting. `NewRouter` chains the four `web` ones around
  the whole mux and mounts the two rate limiters per group;
  `ARCHITECTURE-LIMITATIONS.md` records what no test proves about either.

  **The line between the two packages is whether the middleware knows a caller
  identity or a feature module.** `web` may import `platform/config`,
  `platform/logger` and `platform/response` and nothing else of ours — check 8
  enforces that, and it is what makes the package copyable into a fresh
  project. Anything that reads or writes the user in the request context, or
  names a module type, belongs in `internal/server/middleware`.
  `ratelimit.go` is the instructive case: it looks entirely generic — a Redis
  counter and a window — but it calls `GetUserContext` to key the limit per
  user, so it stays on the server side. A new compression or timeout
  middleware would go in `web`; a new one that reads a role, a tenant or a
  module's config goes beside `auth.go`. If a middleware would need an import
  `web` may not have, that is the answer, not a reason to widen check 8.
- Field exposure controlled by DTO omission, not by `json:"-"`. **There are zero `json:"-"` tags under `internal/`, and check 1b keeps it that way.** Fourteen of them used to be load-bearing security controls (`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`) where deleting two characters published a password hash. Rule 1 exists for that reason: adding a field to a response now means naming it in a DTO deliberately. The failure mode that replaced it is naming the *wrong* DTO — see `ARCHITECTURE-LIMITATIONS.md` on the public and admin response mappers that live in separate, differently-named files (`response.go`, `admin_response.go`) in the same package: the risk narrowed from mistyping an adjacent identifier to reaching for the wrong file, but a handler can still call either mapper and compile.

## Guardrails

- Never hand-edit generated `mocks_test.go` — regenerate with `make mocks`.
- Never commit `.env`, secrets or API keys.
- Run `make check-boundaries`, `make vet` and `make test` before calling change complete. `make all` does all three plus lint and build.
- Do not add third-party router.
- Do not suppress lint or vet findings with `//nolint` without a justification comment on the same line — see `internal/modules/order/service.go:56` (`//nolint:gocognit,funlen // one order write: idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction`) for the expected form. Five methods carry one: `order.Service.Place` and `order.Service.cancelWithReversal`, and `payment.Service.FinalizeSuccess`, `runRefund` (unexported now — it runs from `RefundJob.Run`, not a dispatcher) and `HandleWebhook`. `internal/server/router.go`'s `NewRouter` carries a `//nolint:funlen` for the same kind of reason — one flat wiring list, not nested logic.
- Do not make subpackage tree uniform, and do not add pass-through adapter package to fill slot.
- Backward compatibility explicitly **not** a goal here. API shapes may change where better design demands — but say so when they do.
- When adding a module: create `internal/modules/<feature>/` per the shape under "Inside a module" above — `domain/` for its aggregate, `service.go` declaring one exported `Service` with its `Deps` and `New`, `repository.go` for the storage port, `adapter/postgres/` where it has SQL, `adapter/http/` where it has a route, `ports.go` only if it consumes something from another module, `contract.go` only if a struct of its own has to cross a port. Add a row per owned table to `db/OWNERSHIP.md`. Wire it into `internal/bootstrap/app.go` — one line to build it, one field on `App` — by name-match if an existing port already fits. Mount its routes in `internal/server/router.go`, and add a line per route to `internal/server/testdata/routes.golden`.
- **Adding one route touches three files**: the module's `adapter/http` for the handler, `internal/server/router.go` for the URL, and `internal/server/testdata/routes.golden` for the proof. The golden is not generated: `TestRouteSnapshot` iterates the golden and probes each line, so a route you mount and forget to add is untested rather than failing. Then run `make check-boundaries` — a new module with an `adapter/postgres` and no ownership row fails it by design.

## Further reading

- `README.md` — endpoint reference and quick start. Its "Project Structure" section agrees with this file; both rewritten against real tree. Its environment table is **curated subset** — 8 variables absent, including the whole Redis pool group (`REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`, `REDIS_DIAL_TIMEOUT`, `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT`, `REDIS_POOL_TIMEOUT`) and the worker's prune settings (`WORKER_PRUNE_AGE`, `WORKER_PRUNE_LIMIT`). `.env.example` is the exhaustive list; verified against `envconfig` tags across `internal/platform/config/config.go` (infra) plus each module's own `config.go` (`auth`, `cart`, `notification`, `order`, `payment` — the five with env vars of their own).
- `ARCHITECTURE.md`, `ARCHITECTURE-LIMITATIONS.md`, `db/OWNERSHIP.md` — as above.
- `db/migrations/` — goose SQL migrations.
- `.env.example`, `.mockery.yml`, `.golangci.yml`.
