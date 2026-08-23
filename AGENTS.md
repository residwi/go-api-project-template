# AGENTS.md

Orientation for agents and humans in this repo. Describes tree as it actually is, commands that actually exist, and — most useful — which rules **machine-checked** vs which only convention.

Three docs carry the reasoning; this one no duplicate:

- **`ARCHITECTURE.md`** — seventeen decisions that shaped this codebase, fifteen things it deliberately not do, each with cost. Decision 14 is marked **reversed** and kept as history: it is why this tree held 226 packages of vertical slices for a year, and decision 16 records what replaced it.
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
  mockserver/             its handlers, importable so internal/server can mount them in-process
internal/
  apperror/               error vocabulary (ErrNotFound, ErrBadRequest, ...); no feature deps
  bootstrap/              the composition root: builds every Service and wires every
                          cross-module port by name-match
  server/                 server.go (Run, NewRouter, health), routes.go, middleware/,
                          response/. routes.go holds every URL in the system -- all 64
                          of them, in one function
  platform/               generic infrastructure, no feature deps:
                          cache/ config/ database/ jobs/ logger/ paging/ slug/ storage/ validator/
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
  `internal/apperror` stayed above the module tree for the opposite reason: it
  is an error **vocabulary**, used by `platform` and `server` too, so it is no
  module's to own.

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
  service.go         one exported Service, plus its Deps struct and New.
                     15 of 16 -- money has none, being a value object
  repository.go      the storage port adapter/postgres satisfies (13)
  ports.go           every cross-module port this module consumes, one file (9)
  contract.go        the struct types another module may name (7)
  config.go          this module's own env vars (4: auth cart order payment)
  domain/            aggregate types and rules -- private, and check 4
                     enforces it (14: all but checkout and money)
  service_test.go    mock-driven tests, package <feature>
  mocks_test.go      mockery output, in-package, invisible outside the tests
  adapter/
    postgres/        SQL adapter, where the module has SQL (13)
    http/            handlers plus their wire DTOs (15)
    redis/           user only -- the store behind its StatusCache port
    jobs/            payment only -- the Dispatcher routing a claimed job
```

Re-run the numbers rather than trust them:

```bash
ls -1 internal/modules | wc -l                 # 16 directories
ls internal/modules/*/service.go | wc -l       # 15 services
ls internal/modules/*/repository.go | wc -l    # 13 repository ports
ls internal/modules/*/ports.go | wc -l         #  9 ports files
ls internal/modules/*/contract.go | wc -l      #  7 contract files
ls -d internal/modules/*/adapter/http | wc -l  # 15 http adapters
```

Three directories sit at a module root outside `adapter/`, and none of them
adapts that module's own aggregate:

- **`payment/gateway/`** — the outbound `Gateway` port and its three real
  implementations (`stripe/ midtrans/ mock/`), picked once in
  `payment/service.go`'s `newGateway` from `Config.Gateway`. Check 1's
  json-tag allowlist names `gateway/gateway.go` alone, not the directory,
  because none of the three implementations declares a json-tagged type.
- **`payment/jobs/postgres/`** — the payment job queue's own store, and the
  only Go under `payment/jobs/`. The port it satisfies (`JobRepository`) and
  the `Claim`/`Prune` pair that make `*payment.Service` a
  `platform/jobs.Queue` both live in `payment/jobs.go`, in the module's root
  package.
- **`notification/jobs/`** — `jobs.Worker`, queue and processor at once, plus
  its own `postgres/`. `notification.Service.Jobs` exports it, because `order`
  name-matches its port against it and `cmd/worker` hands it to `jobs.Runner`
  as both halves.

#### Naming

**One `Service` per module, and its methods carry the verb.** `grep -rn
Execute --include='*.go' internal/modules` returns nothing. Four rules came
out of the flatten, and they are worth knowing before adding a method:

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
one file per module, nine files, 29 interfaces** (`grep -c '^type .*
interface' internal/modules/*/ports.go`). The consumer names the interface;
the producer never publishes it. `category/ports.go` declares `ProductCounter`
(`CountPublished`, one method). `order/ports.go` declares eight and
`payment/ports.go` seven. A module that reaches nothing outside itself has no
`ports.go` at all — `dashboard inventory money notification promotion user
wishlist`, seven of the sixteen.

Two mechanisms satisfy a port without an adapter, and `internal/bootstrap` is
the one place either is used:

- **Name-match.** The producer's own value already has a method named what the
  consumer's port asks for. `promotion.Service` satisfies both
  `order.CouponReserver` (`Reserve` + `Release`) and `payment.CouponPort`
  (`Release` alone) — two differently-shaped interfaces, one producer value,
  no adapter for either. `notification/jobs.Worker` satisfies
  `platform/jobs.Processor` directly. `*order.Service` satisfies ten port
  fields across four consumers.
- **A `contract.go` type**, when what crosses is a struct rather than a scalar
  or something a producer already satisfies by name.

**One port per capability, not one wide port per producer.** `shipping`
declares `OrderGetter`, `OrderShipper` and `OrderDeliverer` — three ports, all
three wired to the same `*order.Service` — because a port names what its
caller asks for, not what the producer happens to offer. No `Service` carries
a forwarding method for another module's benefit.

That has a price the sliced shape did not pay. When three ports bound to three
*different* slice values, pasting one into the wrong `Deps` field was a
compile error; one flat `Service` satisfying all three means the swap compiles.
`ARCHITECTURE-LIMITATIONS.md` prices it and counts what is exposed.

A `dto.go` belongs nowhere at all: check 1c refuses that filename **anywhere**
under `internal/`. Wire types live in the module's own `adapter/http`, in the
file that serialises them.

#### `contract.go`

**Seven of the sixteen have one** — `auth cart inventory order payment product
user`. It holds the struct types another module's port names in a signature:
`user.Profile` and `user.Credentials`, `cart.Snapshot`, `order.Snapshot`,
`product.Info`, `inventory.Availability` and `inventory.StockState`,
`auth.ClaimsView`, `payment.ChargeRequest`. A module earns one only when a
*struct* has to cross a port — not a scalar, and not something a producer's
`Service` already satisfies by name. The nine without one never pass a struct
across, which is why they have nothing to show.

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
`handler.go` is the default (public or authed) handler; `admin_handler.go`,
`webhook_handler.go`, `cancel_handler.go` and `retry_handler.go` are the
qualified exceptions. `routes.go` never appears here, or anywhere under
`internal/modules/` — every URL lives in `internal/server/routes.go`, see rule
10. Seven modules carry an `admin_handler.go` beside a `handler.go` because
they split their own routes by caller role (`category order product promotion
review shipping user`); `payment`'s only public route is the gateway callback,
so it has `admin_handler.go` and `webhook_handler.go` and no `handler.go` at
all.

**The route methods on those handlers are exported**, and that is not
cosmetic: `internal/server/routes.go` is a different package in a different
tree, and it can only mount `orderHandler.List`, `userAdminHandler.Get`,
`placeHandler.Place` if it can name them.

The port a handler takes is declared locally, in the handler's own package,
and is named for the role it needs — 25 interfaces across the 15 `adapter/http`
packages, none of them called `UseCase`
(`grep -hoE '^type [A-Za-z]+ interface' internal/modules/*/adapter/http/*.go`).
`CartManager`, `ProductReader`, `WebhookProcessor`, `Reporter`. Eight packages
hold two ports and `checkout` holds three, which is what role naming buys:
naming both `UseCase` would redeclare an identifier. See rule 18a.

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

Four carve-outs put a test outside the package it tests — two forced by an
import cycle, one by preference, one because the thing under test is not Go —
and together they are the whole exception: **13 external test files**
(`grep -rl '^package .*_test$' --include='*_test.go' . | wc -l`).

- `test/e2e` (9 files, `package e2e_test`) imports `internal/bootstrap` and
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

`scripts/check-boundaries.sh` registers **five** checks, and every rule below
names the function that enforces it so the two cannot drift apart by name
alone. **They are numbered 1, 2, 3, 4 and 6. The gaps are deliberate.** Checks
5 and 7 were retired, and renumbering the survivors would falsify every
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
   remove it and check 1 reports **295** tags in fifteen adapters at once.
   Also checked: `json:"-"` must not appear anywhere under `internal/` outside
   an `adapter/http` (no exemption at all, tests included — there are **zero**
   in the tree today), and no file named `dto.go` may exist anywhere under
   `internal/`. Two paths are allowlisted by name _with a stated reason_ in
   the script — `internal/modules/payment/gateway/gateway.go` (the external
   gateway's wire contract, not ours) and
   `internal/server/response/response.go` (the shared envelope every handler
   writes through) — plus `internal/platform/` by location, which covers
   `internal/platform/config/`: those are `envconfig` tags, not `json`, but
   the exemption matters so that adding one is not mistaken for a domain leak.
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
   right. **One per-target exemption exists: `checkout` alone may import
   `order/domain`**, because `order.Service.Place`'s signature is written in
   `orderdomain.NewOrder` and `*orderdomain.Order` and `order/contract.go`
   publishes neither. Removing that exemption reports **7** violations, not
   zero. It is a real weakening of the rule for one module of sixteen, and
   `ARCHITECTURE-LIMITATIONS.md` names it as its own limitation.
5. **RETIRED — `check_sibling_slice_imports`.** It refused a slice importing a
   sibling slice inside its own module, a rule check 4 structurally could not
   see. With no `usecase/` tree left anywhere it walked nothing and could only
   pass, so it was deleted along with its probe. Nothing replaces it: the
   coupling it prevented needed two peer packages inside one module, and there
   is one `Service` per module now. The number is left vacant on purpose.
6. **`check_transport_direction`: a module may not import `internal/server`,
   except its own `adapter/http`.** That is the one exempt location, and it is
   load-bearing rather than decorative: every module's `adapter/http` calls
   `response.Bind` and `middleware.RequireUser` (rule 18), so removing the arm
   reports **85** imports across **fifteen** modules in one run. The check
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

`scripts/boundaries_test.go` is what keeps a path-keyed check from dying
quietly. It plants a probe file in a real module — a json tag, a `dto.go`, a
foreign `FROM orders`, a `domain/` import, an adapter import, an
`internal/server` import — and asserts the script reports each one. It also
probes from the other side, asserting that a module's root-package import, the
wiring layer's adapter import and `checkout`'s `order/domain` import all stay
clean, so an exemption that has stopped matching anything fails a test instead
of printing `Boundaries OK`. Run it with `go test ./scripts/`.

Two more rules are machine-checked, but by `make lint` rather than
`make check-boundaries` — which means `make ci` catches them and
`check-boundaries` does not:

8. **No stdlib `log`, anywhere.** `depguard` denies `pkg: log$` across
   `$all`. There is no `main.go` carve-out: `Run` and `run` report their own
   failures, so `main` needs no logger of its own and holds only the exit
   code.
9. **No `slog.Any`, anywhere.** `forbidigo` denies the identifier. Every
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
    `internal/server/routes.go` — one `registerRoutes` function, 64 routes,
    fifteen labelled blocks — which `NewRouter` in `internal/server/server.go`
    calls once. A module supplies a handler with exported route methods; the
    transport decides the verb, the path, and which `middleware.RouteGroup` it
    lands on. Check 6 enforces the direction; `ARCHITECTURE.md` decision 15 is
    why, and its cost.

    Two mechanisms satisfy a port without an adapter:
    - **Name-match.** The producer's own value already has a method named what
      the consumer's port asks for. `promotion.Service` satisfies both
      `order.CouponReserver` (`Reserve` + `Release`) and `payment.CouponPort`
      (`Release` alone) directly — two differently-shaped interfaces, one
      producer value, no adapter for either. `notification/jobs.Worker`
      satisfies `platform/jobs.Processor` directly. `*order.Service` satisfies
      ten port fields across four consumers — `payment`'s `OrderTransition`,
      `OrderCanceller` and `OrderReader`, `checkout`'s `Orders`, `Snapshots`
      and `Cancels`, `shipping`'s `OrderRead`, `OrderShip` and `OrderDeliver`,
      and `review`'s `Purchase` — and `internal/bootstrap/app.go` hands the
      same value to all ten. A consumer still declares one port per capability
      rather than one wide one, because a port names what its caller asks for.
    - **A `contract.go` type**, when what crosses is a struct rather than a
      scalar or an interface a producer already satisfies. The consumer's port
      still names the type it needs (`checkout.OrderSnapshotReader.Snapshot`
      returns `order.Snapshot`); `contract.go` supplies only the shape, never
      the interface.

    No shared ports package, and adding one would defeat the point.
11. **Services take `database.TxRunner`, never `*pgxpool.Pool`.** Service needs atomicity, not DB handle. `TxRunner` declared once in `internal/platform/database` not per consumer — one deliberate exception to rule 10's consumer-declaration pattern, because modules already import `platform/database`. A module that opens no transaction takes no runner at all: five `Service`s hold one (`cart order payment promotion shipping`).
12. **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope is four features: `order`, `payment`, `product`, `cart`. `promotion` and `dashboard` stay on `int64` for stated reasons — `ARCHITECTURE.md` §10 and `ARCHITECTURE-LIMITATIONS.md`. `Money` carries no `json` tag and implements no `sql.Scanner`: each adapter maps it explicit, because wire shapes genuinely differ per endpoint. No float constructor and no `Div`.
13. **A `Service` runs no SQL and holds no pool.** Every read and write goes through the module's own `Repository` interface; `adapter/postgres` owns the pool and reaches it with `database.DB(ctx, pool)`, which returns the context's transaction if there is one. A `Service` composes several repository calls into one unit of work via its `TxRunner`, and the transaction propagates to every repository it touches — its own and other modules' — through `ctx`. **`internal/bootstrap` is what constructs the adapters**: `bootstrap.New` builds `orderpg.New(d.Pool)` and hands it to `order.New` as `Deps.Repo`, so the pool never reaches a `Service` and there is no `module.go` left to hide the wiring in. Two `Service`s take a raw `*pgxpool.Pool` anyway, and both are named exceptions: `payment.Deps.Pool` and `notification.Deps.Pool` exist so each can build its own job-queue adapter (`payment/jobs/postgres`, `notification/jobs/postgres`), which `bootstrap` does not name.
14. **Order status changes only through `order.Service.Apply`.** Every guarded transition is a named `domain.Transition` value in `internal/modules/order/domain/transition.go` (`PaidTransition`, `RefundTransition`, `CancelledTransition`, …). Other modules depend on _intent_ methods on their own port interface (`payment.OrderTransition.MarkPaid`; `shipping.OrderShipper.MarkShipped` and `shipping.OrderDeliverer.MarkDelivered`, two ports rather than one because `Create` and `Deliver` are two different callers asking for two different intents), and every caller wires to `order.Service`, since `Apply` and the eight `Mark*` methods that forward to it live there and nowhere else. Never write an ad-hoc from/to status list at a call site.
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
16. **Background job workers use `platform/jobs`.** The value draining a queue implements `jobs.Queue[T]` (`Claim` + `Prune`) and `jobs.Processor[T]` (`Process`) on whatever the binary hands `jobs.Runner[T]`, plus an optional `jobs.Sweeper` for per-tick housekeeping. Two queues exist: `*payment.Service` itself — `Claim` and `Prune` are methods on it, in `payment/jobs.go`, so no separate `Queue` type stands between the Service and the runner — and `notification/jobs.Worker`, which is queue and processor at once. `payment/adapter/jobs.Dispatcher` is payment's processor, routing a claimed job to `Service.RunChargeJob` or `Service.RunRefundJob`. The per-tick sweep that adds order's stale-order housekeeping on top of it is `cmd/worker/main.go`'s own unexported `paymentProcessor`, not a package under `payment`: the composition crosses payment and order, so it belongs at the root that already imports both. Never hand-roll a ticker/lease/poll loop — the runner owns polling, leased compare-and-set claim, bounded concurrency, per-job timeouts and pruning.
17. **Repository reads use `pgx.CollectRows`**, never a hand-rolled `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and build keyset predicates with `database.KeysetCursor()`.
18. **Handlers use the shared helpers.** Decode and validate with `response.Bind[T](w, r, h.validator)`; read caller with `middleware.RequireUser(w, r)`; return errors through `response.HandleErr`. Do not hand-roll decode/validate or auth-context blocks.
18a. **An `adapter/http` port is named for the role it plays, never for the
    pattern.** `CartManager`, `ProductReader`, `WebhookProcessor`,
    `PromotionApplier`, `Reporter` — 25 ports across the 15 `adapter/http`
    packages, no exceptions
    (`grep -hoE '^type [A-Za-z]+ interface' internal/modules/*/adapter/http/*.go`).
    Never `UseCase`, and never `Service` either: the port is what the *handler*
    asks for, which is a subset of what the module's `Service` offers, and
    reusing the producer's name would say otherwise. Eight packages hold two
    ports and `checkout` holds three, because they split routes by caller role
    — `user` has `ProfileManager` + `UserManager`, `product` has
    `ProductReader` + `ProductManager`, `checkout` has `OrderPlacer` +
    `PaymentRetrier` + `OrderCanceller`. Role naming is what lets them
    coexist; one shared name would redeclare an identifier.

    **The `Handler` field holding that port is `service`, and so is the
    constructor parameter that sets it**: `h.service.PlaceOrder(...)`, in all
    25. The constructors are `NewHandler` (14), `NewAdminHandler` (8), and one
    each of `NewWebhookHandler`, `NewCancelHandler` and `NewRetryHandler`. The
    old `usecase` field name and bare `New` constructor retired with the
    slices. A field is private and there is only ever one, so it is named for
    the layer, not the role — never `cmd`, `reader` or `svc`.
19. **New config invariants go in the owning type's own loader.** Infra-level
    invariants go in `Infra.validate()` (`internal/platform/config/config.go`);
    module-owned invariants are checked inline inside that module's own
    `LoadConfig` (`auth.LoadConfig`, `cart.LoadConfig`, `order.LoadConfig`,
    `payment.LoadConfig`), since each module loads its own env vars now and
    there is no longer one central `Config.validate()` for every invariant to
    share. Either way, misconfiguration aborts boot instead of surfacing later
    as a runtime error. Do not guard per use site.
20. **Request-scoped attributes are named once, at the edge.**
    `logger.WithAttrs(ctx, ...)` stashes them and `logger.ContextHandler`
    merges them into every record below, so no function grows a parameter
    to carry `request_id`. Four edges do this: `middleware.RequestID`
    (`request_id`), `middleware.Auth` (`user_id`), `jobs.Runner.Start`
    (`runner`), and each queue-draining `Process` (`job_id`).
21. **An attribute may only be named at an edge that owns exactly one
    value.** `order_id` and `payment_id` stay written at the call site
    because a command loops over batches of orders — one context
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

## Git history

History stays linear. No `Merge branch …` commit — it says nothing about the code and makes `git log` a graph to decode instead of a list to read.

- **Rebase feature branch onto `main` before integrating**, then merge fast-forward:
  `git switch main && git merge --ff-only <branch>`. If fast-forward refuses, branch is behind — rebase again, do not fall back to a merge commit.
- **`git pull --rebase`, never plain `git pull`.** Set it once so it cannot be forgotten: `git config pull.rebase true`.
- Rebase early and often while branch is alive. Cost of rebasing scale with how far the two lines have drifted, so daily is cheap and monthly is not.

**Rebase only works on history that is already linear.** It replay commits in one sequence, so any merge it cross disappear and the two sides interleave into an order that never existed — a commit then land on code it was never written against, and conflict cascade for the rest of the replay. Three merges predate this rule; do not rebase across them.

## Testing

- `testing` + `stretchr/testify`. `require` when test cannot continue without value, `assert` for soft checks.
- **Docker is required.** No build tags, no short mode. `internal/testutil` starts two long-lived containers by fixed name (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to whichever already exists. Remove them with `make test-clean`.
- **SQL semantics stay in the adapter's own test.** Recursive CTEs, keyset pagination, unique constraints — anything only the database can prove — belong in `internal/modules/<feature>/adapter/postgres/repository_test.go` (or `adapter/redis/cache_test.go`) against a real container. Anything a mock can express — a `Service`'s reaction to a value, an error branch — belongs in that module's `service_test.go` instead, and a saga spanning tables no single module owns goes to `test/e2e/`. No module starts its own container. `go test ./...` runs package binaries concurrently; collapsing per-package tests into one `test/integration` package would make them sequential. `ARCHITECTURE.md` decision 11 rejects that directory explicitly.
- **`test/e2e/` is for sagas no single module can own** — checkout, payment, refund, fulfilment failure, admin flows — driven through the real `server.NewRouter`, real Postgres, and the mock gateway on an `httptest.Server`.
- **Postgres databases are per module, created once under an advisory lock, and never dropped; Redis indices are still a slot you claim.** `MustStartPostgres(dbName)` creates and migrates `dbName` the first time any caller asks for it — the lock covers the migration too, so a second caller that finds the database already there finds it at the latest schema — and every later caller, same test binary or a different one, just connects. **25 packages call `testutil.MustStart*` today** (`grep -rl 'testutil.MustStart' --include='*_test.go' . | xargs -n1 dirname | sort -u | wc -l`), and the mapping is one database per module: `test_cart`, `test_order`, `test_payment`, and so on (`grep -rn 'MustStartPostgres(' --include='*_test.go' internal/modules` is the live mapping). Two modules still put two test packages on one name — `notification` and `payment` each have an `adapter/postgres` and a `jobs/postgres` test package sharing `test_notification` / `test_payment`. Packages sharing a name never tear each other down, but they get no clean table between them either: seed the rows your subtest asserts on and never `TRUNCATE`. **`ResetDB` is safe only for a package that owns its database outright, and nothing under `internal/modules` does.** Three callers today: `internal/bootstrap/app_test.go` (`test_bootstrap`), `internal/server/router_test.go` (`test_server`) and `test/e2e/testmain_test.go` (`test_e2e`). `ResetDB` takes a `*pgxpool.Pool`, not a package name, so nothing stops a fourth caller inside a module adding it — check before copying a `setup` helper that calls it. `MustStartRedis(dbIndex)` takes an index the caller picks by hand against the registry comment above that function in `internal/testutil/testutil.go`. Indices 0, 1, 3, 5 and 6 are claimed (`platform/cache`, `server/middleware`, `server`, `test/e2e`, `modules/user/adapter/redis`); 2 and 4 are free. Nothing enforces a claim — a collision compiles, passes review, and fails as a flake in an unrelated package — so update that comment in the same commit that takes an index, and cross-check it against `grep -rn 'MustStartRedis(' --include='*_test.go' .`, which is the only record that cannot drift.
- **`t.Parallel()` buys nothing in a package that owns a database or a Redis
  index**, because everything in that package shares one connection and `ResetDB` TRUNCATEs every table in it. Those packages excluded from `paralleltest` wholesale in `.golangci.yml` -- per package, never per file, because parallel sibling gets its rows deleted mid-assertion even when that sibling never calls reset itself. That exclusion's `path:` regex is **anchored** (`^internal/...`), so it dies silently the day one of the directories it names moves -- check it against `git ls-files` after any structural change. The two unanchored `path:` patterns in that file (`cmd/*` and `cmd/|testutil/`) survive a move by accident rather than by edit, which is the distinction to know when auditing the list. Nothing given up: `go test` already runs packages concurrently and each owns own database. Have each subtest seed own data instead.
- **Everywhere else `t.Parallel()` is mandatory**, and `paralleltest` enforces it on both test function and every `t.Run` closure. If you add test package claiming database or Redis slot, add it to that exclusion list in same commit.
- **Order a test file so the tests come first.** Package-level `var`s and `TestMain` at top, then every `func TestXxx`, then stub types with their own methods grouped under them, then plain helpers last. `internal/platform/jobs/runner_test.go` is the shape. Someone opening file came for scenarios, not fakes that serve them. `funcorder` only orders methods against their struct, so nothing lints the rest — on you.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest, descriptive name, own setup. Break large scenarios up; no monolithic tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
result)` on full struct or slice. For JSONB round-trips use `assert.JSONEq` — Postgres normalises whitespace.
- **Test behaviour, not wiring.** Verify returned value, error, or side effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` **in-package**, as `mocks_test.go` beside the interface they mock. A `_test.go` file never enters its package's importable `GoFiles`, so a mock is private to that package and cannot cycle back — which lets a module's own `service_test.go` stay `package <feature>` and its `handler_test.go` stay `package http`, each mocking only the interfaces declared in its own package, and keeps every `Mock*` name out of the module's exported API. 38 `mocks_test.go` files exist (`find internal -name mocks_test.go | wc -l`). `.mockery.yml` is one recursive rule rooted at `internal/`, `all: true`, no per-interface `configs:` list — every interface gets exactly one mock, in its own package. This is why an `adapter/http` handler declares its own narrow port locally rather than importing one from the module's root package: mockery cannot write a private mock into a package that does not declare the interface, so an interface consumed across a package boundary has to be declared on the consumer's side either way. Run `make mocks`; never hand-edit the generated file. Use the expecter API (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`), never `repo.On("GetByID", ...)`.
- **Keep tests fast.** Use `bcrypt.MinCost` for password hashes in tests (`DefaultCost` costs ~250ms per hash) and group tests exercising real `Register` path. Use `testing/synctest` for ticker- and timeout-driven code — `internal/platform/jobs/runner_test.go` does. Note `synctest` cannot wrap `pgxpool` acquire, so test holding real pool must shrink intervals and timeouts instead. Give intentionally-broken clients short timeouts (`MaxRetries: 0`, `DialTimeout: 200 * time.Millisecond`) so error paths fail in milliseconds not seconds.

## Security

- Secrets come from env vars or gitignored `.env`. Never commit real secrets. `.env.example` lists every supported variable.
- JWT auth with configurable expiry; bcrypt password hashes; RBAC via admin middleware.
- Middleware in `internal/server/middleware/`: panic recovery, request-ID injection, structured request logging, CORS, rate limiting, auth, admin. `NewRouter` chains four of them around the whole mux and mounts the two rate limiters per group; `ARCHITECTURE-LIMITATIONS.md` records what no test proves about either.
- Field exposure controlled by DTO omission, not by `json:"-"`. **There are zero `json:"-"` tags under `internal/`, and check 1b keeps it that way.** Fourteen of them used to be load-bearing security controls (`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`) where deleting two characters published a password hash. Rule 1 exists for that reason: adding a field to a response now means naming it in a DTO deliberately. The failure mode that replaced it is naming the *wrong* DTO — see `ARCHITECTURE-LIMITATIONS.md` on the two response mappers that now sit one identifier apart in the same package.

## Guardrails

- Never hand-edit generated `mocks_test.go` — regenerate with `make mocks`.
- Never commit `.env`, secrets or API keys.
- Run `make check-boundaries`, `make vet` and `make test` before calling change complete. `make all` does all three plus lint and build.
- Do not add third-party router.
- Do not suppress lint or vet findings with `//nolint` without a justification comment on the same line — see `internal/modules/order/service.go:70` (`//nolint:gocognit // one order write: idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction`) for the expected form. Five methods carry one: `order.Service.Place` and `order.Service.cancelWithReversal`, and `payment.Service.FinalizeSuccess`, `RunRefundJob` and `HandleWebhook`. `internal/server/routes.go` carries a `//nolint:funlen` for the same kind of reason — one flat wiring list, not nested logic.
- Do not make subpackage tree uniform, and do not add pass-through adapter package to fill slot.
- Backward compatibility explicitly **not** a goal here. API shapes may change where better design demands — but say so when they do.
- When adding a module: create `internal/modules/<feature>/` per the shape under "Inside a module" above — `domain/` for its aggregate, `service.go` declaring one exported `Service` with its `Deps` and `New`, `repository.go` for the storage port, `adapter/postgres/` where it has SQL, `adapter/http/` where it has a route, `ports.go` only if it consumes something from another module, `contract.go` only if a struct of its own has to cross a port. Add a row per owned table to `db/OWNERSHIP.md`. Wire it into `internal/bootstrap/app.go` — one line to build it, one field on `App` — by name-match if an existing port already fits. Mount its routes in `internal/server/routes.go`, and add a line per route to `internal/server/testdata/routes.golden`.
- **Adding one route touches three files**: the module's `adapter/http` for the handler, `internal/server/routes.go` for the URL, and `internal/server/testdata/routes.golden` for the proof. The golden is not generated: `TestRouteSnapshot` iterates the golden and probes each line, so a route you mount and forget to add is untested rather than failing. Then run `make check-boundaries` — a new module with an `adapter/postgres` and no ownership row fails it by design.

## Further reading

- `README.md` — endpoint reference and quick start. Its "Project Structure" section agrees with this file; both rewritten against real tree. Its environment table is **curated subset** — 8 variables absent, including the whole Redis pool group (`REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`, `REDIS_DIAL_TIMEOUT`, `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT`, `REDIS_POOL_TIMEOUT`) and the worker's prune settings (`WORKER_PRUNE_AGE`, `WORKER_PRUNE_LIMIT`). `.env.example` is the exhaustive list; verified against `envconfig` tags across `internal/platform/config/config.go` (infra) plus each module's own `config.go` (`auth`, `cart`, `order`, `payment` — the four with env vars of their own).
- `ARCHITECTURE.md`, `ARCHITECTURE-LIMITATIONS.md`, `db/OWNERSHIP.md` — as above.
- `db/migrations/` — goose SQL migrations.
- `.env.example`, `.mockery.yml`, `.golangci.yml`.
