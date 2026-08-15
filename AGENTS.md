# AGENTS.md

Orientation for agents and humans in this repo. Describes tree as it actually is, commands that actually exist, and — most useful — which rules **machine-checked** vs which only convention.

Three docs carry the reasoning; this one no duplicate:

- **`ARCHITECTURE.md`** — sixteen decisions that shaped this codebase, fifteen things it deliberately not do, each with cost.
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
  bootstrap/              the composition root: builds every service, wires cross-module
                          ports by name-match, breaks the order/payment cycle after construction
  transport/http/         server.go, router.go, routes/, middleware/, response/
                          routes/ holds every URL in the system: one file per
                          feature, 14 of them, mounting that feature's slices
  platform/               generic infrastructure, no feature deps:
                          cache/ config/ database/ jobs/ logger/ paging/ slug/ storage/ validator/
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
`apperror bootstrap money platform testhelper transport`.
`scripts/check-boundaries.sh` derives feature list structurally, reading directory names under `internal/modules/`, so adding feature enough to enrol it in boundary checks; no denylist to remember.
`scripts/check-boundaries.sh` now runs **seven** checks (see Machine-checked, below). Being infrastructure exempts a directory from checks 2 and 3's _ownership_ questions — they loop over `feature_dirs`, i.e. `internal/modules/*` only, so `internal/platform` is never in their scan — but not from check 4: only the wiring layer, `bootstrap` and `transport` (script's `WIRING_DIRS`), may import anything from a module beyond its `contract/`, so `internal/platform` importing `internal/modules/order/domain` fails exactly as another feature importing it would, and so does `internal/platform` reaching a slice two levels deeper, `internal/modules/shipping/usecase/query`. Check 6 (the transport-direction rule) runs the other way and is narrower in scope: it loops over `feature_dirs` only, so it polices whether a *feature* imports `internal/transport`, not whether `internal/platform` does.

### Inside a feature

**Every module is sliced, and every slice lives under `usecase/`. There is no
more layered shape to compare against.**
`ls internal/modules/<feature>/` tells the truth for any of the fourteen now:
a `domain/` directory, one `module.go`, and one `usecase/` directory holding
one package per use case — never a root `model.go`, `service.go` or
`repository.go`, and no root `http/` either: **a module names no URL.**
`shipping` went first (`ARCHITECTURE.md` §14); `payment` went last.

```text
internal/modules/<feature>/
  domain/            aggregate types and rules; module-private by convention,
                     not by any check -- nothing outside the module imports it
  module.go          composes every slice into Module; also declares any
                     cross-module port more than one sibling slice shares
  module_test.go     composition test, where one exists (order, payment) --
                     proves New wires every slice correctly; not every
                     module needs one
  contract/          published struct types, only if another module consumes
                     one (seven of fourteen: auth cart inventory order
                     payment product user)
  config.go          only auth, cart, order, payment: this module's own env vars
  usecase/           holds slices and nothing else
    <slice>/         one package per use case -- see below
```

A slice's own directory holds `usecase.go`, and that file declares exactly one
exported `UseCase` — 66 slices, 66 `usecase.go` files, 66 `type UseCase`
declarations, whatever the slice does. `place.UseCase.Execute` writes;
`shipping/usecase/query.UseCase.GetByOrderIDForUser` only reads;
`order/usecase/transition.UseCase` carries ten methods because a state machine
is one thing with ten guarded entrances. The older per-role names are retired
and none survives anywhere in a slice: no `Command`, `Reader`, `Service`,
`Store` or `Applier`. Beside `usecase.go` sit `repository.go` (the storage
port its own `postgres/` satisfies), `ports.go` (cross-module ports only
this slice needs — absent where a slice reaches nothing outside itself:
`wishlist` has no `ports.go` anywhere in the module), a `postgres/` adapter
where it has SQL, and an `http/` adapter where it has a route of its own.
"Own" is literal — two slices needing the same repository method each
declare it rather than share one.

66 slices exist today, and the recipe no longer carries an exclusion set,
because `usecase/` holds slices and nothing else: `find
internal/modules/*/usecase -mindepth 1 -maxdepth 1 -type d | wc -l`. Re-run
it rather than trust the number. Four directories sit outside `usecase/` at a
feature root, and none of them is a slice. Three are `payment`'s.
`gateway/` bundles the outbound `Gateway` port and its three real
implementations (`stripe/ midtrans/ mock/`), picked once in `module.go` from
`Config.Gateway` — check 1's json-tag exemption names `gateway/gateway.go`
alone, not the directory, because none of the three implementations declares
a json-tagged type of its own. `jobs/` is payment's own queue (`jobs.Queue`)
plus the `jobs.Dispatcher` that routes a claimed job to `charge` or `refund`.
`worker/` wraps that dispatcher plus order's stale-order housekeeping into
the binary's per-tick `Sweep` hook — a behaviour decision predating slicing,
kept deliberately, not a boundary one. The fourth is `notification/jobs/`,
whose `jobs.Worker` is queue and processor at once. None has a `usecase.go`,
none declares a `UseCase`, and `cmd/worker/main.go` constructs
`worker.Processor` directly rather than through `payment.New` — so none is a
slice, whichever way an import into it points.

A cross-module port is declared where it is consumed: in a slice's own
`ports.go` when only that slice needs it
(`category/usecase/remove/ports.go` declares `ProductCounter`;
`product/usecase/query`, `product/usecase/create`, `review/usecase/create`
and 22 more each declare their own the same way — 26 `ports.go` files in the
tree, 25 of them a slice's and one `payment/jobs`', per `find
internal/modules -name ports.go`), or in `module.go` — as an interface plus a
`Deps` field — when
several sibling slices share the dependency. `order`, `payment`, `cart`,
`auth` and `shipping` all do the latter: `order/module.go` alone declares
six (`CartProvider`, `InventoryPort`, `CouponPort`,
`NotificationEnqueuer`, `PaymentInitiator`, `PaymentJobCanceller`), because
`place`, `cancel` and `expire` all need inventory and no one slice owns
that need alone. Either way the rule is the same one decision 2 states —
the consumer names the interface, never the producer — the two just differ
in which consumer that is: one slice, or the module composing several.

A `Module` struct field is named for the capability it backs, not the
package — `category.Module` has field `Delete` backing package `remove`. The
one exception AGENTS.md used to record, `cart.Module.Empty` backing package
`empty`, is gone: the slice is `cart/usecase/clear`, the field is
`ClearCart`, and `cart.Module`'s `Clear(ctx, userID)` delegator — the thing
`order.CartProvider.Clear` actually reaches — is unchanged. That rename cost
one suppression: `clear` is a Go builtin, and `predeclared` fires on the
package clause itself, so `.golangci.yml` carries a `predeclared` exclusion
scoped to `^internal/modules/cart/usecase/clear/` with the exact linter text.
It is the narrowest form available — no file in that package calls the
builtin `clear()` on a map or slice, only the package identifier collides.

A `dto.go` belongs nowhere at all: check 1c refuses that filename
**anywhere** under `internal/`, not just at a feature or slice root. Wire
types live in the slice's own `http/handler.go` (or `admin_handler.go`,
`webhook_handler.go`) that serialises them.

Seven of the fourteen features — `auth cart inventory order payment product
user` — have a `contract/` package: the one place another module may import
a *type* from, as opposed to merely satisfying an interface. Holds only the
structs a consumer's port names in its return type (`user/contract.User`,
`product/contract.Product`, `inventory/contract.StockState`, …), imports no
module and no platform package, so importing it can never pull the
producer's implementation along. A module gets one only when a struct — not
a scalar, not something a producer's service already satisfies by name —
must cross a port; the other seven modules never pass one and have no
`contract/` to show for it. `ARCHITECTURE.md` decision 13 is why, and its
cost.

**Subpackage tree stays non-uniform, now two levels deeper than it used to
be — do not tidy it into uniformity.** A slice gets a `postgres/` only if it
has SQL, an `http/` only if it has a route, a `ports.go` only if it reaches
outside itself. 66 slices carry 62 `postgres/` packages, 53 `http/` and one
`redis/` between them — the tree's other two `postgres/` packages,
`notification/jobs/postgres` and `payment/jobs/postgres`, sit at feature
roots outside `usecase/` and are not a slice's, per decision 16 below.
`auth` has no `postgres/` anywhere in the module: it asks `user` for one
thing (`auth.UserProvider`) and stores nothing of its own.
`user/usecase/query` is the one slice in the repo with two backing
stores, and the only one with a `redis/`: `query.Repository` pairs with
`postgres/`, `query.StatusCache` with `redis/`, one adapter subpackage per
store, same rule decision 5 states for a feature root. That adapter needs
Redis 8.0 or later — built on `HSETEX`, which does not exist on earlier
Redis. `order/usecase/changestatus` has a `http/` with no `handler.go` at
all, because its whole surface is one admin route,
`PUT /api/admin/orders/{id}/status`. A table
enumerating every feature's shape belongs nowhere in this file: three such
paragraphs already went stale during phase 2 by doing exactly that. Read
`ls` for the feature in front of you.

Inside a slice's own `http/`, files split by **handler role**. Unqualified
`handler.go` is the default (public or authed) handler; `admin_` and
`webhook_` are qualified exceptions; `routes.go` never appears here, or
anywhere else under `internal/modules/` — route tables moved out of the
modules entirely (see rule 10). Counted across
`internal/modules/*/usecase/*/http/`, 53 such directories: 51
`handler.go`/`handler_test.go` pairs, 4 `admin_handler.go`/
`admin_handler_test.go` pairs (three of them — `user/usecase/query`,
`order/usecase/query`, `product/usecase/query` — sit beside a `handler.go`
in the same slice, because those three split their own routes by caller
role; the fourth, `order/usecase/changestatus`, has no `handler.go` at all,
per the paragraph above), and one
`webhook_handler.go`/`webhook_handler_test.go` pair, in
`payment/usecase/webhook`. A slice's `http/` with no `handler.go` means
every route on that slice registers on the admin group.

**The route methods on those handlers are exported**, and that is not
cosmetic: `internal/transport/http/routes/<feature>.go` is a different
package in a different tree, and it can only mount `place.Place`,
`query.List`, `adminQuery.Get` if it can name them. That is the whole reason
they are capitalised.

The port a handler takes is declared locally, in the handler's own package,
and **is not uniformly named `UseCase`.** 56 such interfaces exist across the
53 slice `http/` packages; 8 are called `UseCase` and the other 48 are named
for the role the handler needs — `CartAdder`, `ProductCreator`, `UserGetter`,
`ShipmentDeliverer`, and so on
(`grep -hoE '^type [A-Za-z]+ interface' internal/modules/*/usecase/*/http/*.go`).
Uniformity is not available here even in principle: three packages hold two
ports each — `order/usecase/query/http` (`UseCase` + `AdminReader`),
`product/usecase/query/http` (`ProductReader` + `AdminProductReader`),
`user/usecase/query/http` (`UserGetter` + `UserLister`) — and naming both
`UseCase` would redeclare an identifier. Name the port for what the handler
asks of it; do not rename an existing one for symmetry.

**Tests live in package they test, except where import cycle forbids it.** `handler_test.go`, `admin_handler_test.go`, `webhook_handler_test.go` are `package http`, holding both route-level tests driven through mux and leak tests calling unexported mappers (`toProductResponse`, …) direct — tests that stop domain field reaching unauthenticated response body. In-package permits white-box testing without preventing black-box testing, so one file now does both; that dissolves old constraint that split them, which is why separate leak-test file per feature is gone — contents moved beside implementation file declaring what they test.

**A handler test writes its own URL, and that URL is not the production one.**
55 of the 56 test files in slice `http/` packages build their own
`middleware.NewRouteGroup` and register the handler on a prefix they choose
themselves — the sole exception,
`order/usecase/query/http/admin_handler_test.go`, calls the handler directly.
None of them imports `internal/transport/http/routes`, and several write
`/api/v1/...`, a prefix production has never served. So a handler test proves
the handler, never the URL or the group it lands on. **Only
`internal/transport/http/router_test.go` and `test/e2e/` drive the real
`apihttp.NewRouter`, and they are the only things that can catch a route
mounted on the wrong group.** Neither snapshots the whole table; both sample
it (`ARCHITECTURE-LIMITATIONS.md` records this as a blind spot).

Four carve-outs remain — two forced by import cycle, one by preference, one
by the thing under test not being Go — and together they are the whole
exception: 13 external test files, no more
(`grep -rl '^package .*_test$' --include='*_test.go' .`). A slice's
`usecase_test.go` is never among them: mocks generate in-package, so a mock
no longer imports the package it mocks, and every one of those is
`package <slice>` (`package place`, `package add`, …), same as the two
feature-root `module_test.go` files (`order`, `payment`) are
`package <feature>`. `scripts/boundaries_test.go` (1 file, `package
scripts_test`) is the fourth carve-out and the newest: it shells out to
`scripts/check-boundaries.sh`, plants a probe file in a real slice, and
asserts the script reports it — there is no Go package for it to be inside.
`test/e2e` (9 files, `package e2e_test`) imports `internal/bootstrap` and
`internal/transport/http` to drive the real router end to end, plus each
module's own root package where the saga needs its `Config`/`LoadConfig`
(`auth cart order payment`) or a domain type to assert on
(`payment/domain`) — no `postgres` adapter of any feature, now that
`bootstrap.New` is the one place that wires one up. Nothing about the saga
can live in a single feature package without becoming dependent of its
siblings, which `make check-boundaries` forbids. `internal/testhelper/txrunner_test.go` (1 file, `package testhelper_test`) asserts `database.TxRunner` satisfied from outside `testhelper`, which cannot import `platform/database` itself — `platform/database`'s own in-package tests import `testhelper` for `MustStartPostgres`, so dependency can only run other way in external file. The preference carve-out now holds two files. `internal/bootstrap/app_test.go` (`package bootstrap_test`) is the original: its own doc comment says it exercises `bootstrap.New` from outside "the way every real caller does", which is a preference, not a cycle — neither `internal/testhelper` nor `internal/modules/order/usecase/place/postgres` imports `internal/bootstrap`, so an in-package test could have used them freely. `internal/testhelper/testhelper_test.go` (`package testhelper_test`) joins it for the same reason: it calls nothing but the exported `MustStartPostgres`, no unexported identifier and no import that would cycle back, so it too could have run in-package and chose the outside view instead. Go's own standard library draws per-file line same way: `net/http` ships 19 in-package test files (`package http`) beside 18 external `package http_test` files in same directory, choosing per file by access test needs, not one package-wide policy. Put new test where its access requires, then name file for that.

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

`scripts/check-boundaries.sh` registers seven checks, in this order, and every
rule below names the function that enforces it so the two cannot drift apart
by name alone:

1. **`check_wire_tags`: a `json` tag lives only in a slice's own `http/`.**
   Exempt location is `internal/modules/<feature>/usecase/<slice>/http/*.go`,
   and nothing shorter: `internal/modules/<feature>/http/` is deliberately
   **not** exempt, because that path held the feature route tables until they
   moved to `internal/transport/http/routes/`, and a json tag reappearing
   there would mean a DTO had drifted out of the slice that owns it.
   `internal/transport/http/` is not exempt either — the router wires slices
   and declares no wire type of its own. Also checked: `json:"-"` must
   not appear anywhere under `internal/` outside a slice's `http/` (no
   exemption at all, tests included), and no file named `dto.go` may exist
   anywhere under `internal/`. Exemptions allowlisted by path _with stated
   reason_ in the script — `internal/modules/payment/gateway/gateway.go`
   (the external gateway's wire contract, not ours) and
   `internal/transport/http/response/response.go` (the shared envelope
   every handler writes through) — plus `internal/platform/` by location,
   which covers `internal/platform/config/` too: `envconfig` tags, not
   `json`, but the exemption matters so adding one is not mistaken for a
   domain leak.
2. **`check_ownership_doc`: `db/OWNERSHIP.md` itself has no duplicate row,
   no row for a table no migration creates, and no table with no owning
   row.** Parsed out of the document at run time, between the BEGIN/END
   OWNERSHIP TABLE markers — the document and the check read the same
   list, so they cannot drift.
3. **`check_table_ownership`: a module's SQL only names tables it owns.**
   Scans **every non-test `.go` file under the module**, not only a
   `postgres/` directory — a query in a slice's `usecase.go` or `module.go`
   itself is caught the same as one in `usecase/<slice>/postgres/`. Keywords:
   `FROM`, `JOIN`, `INSERT INTO`, `UPDATE`, `TRUNCATE`, `COPY`, matched
   across newlines and through quoted identifiers. A module is skipped only
   when it has no `postgres/` directory anywhere under it (a legitimate
   no-storage feature, e.g. `auth`). Only a match against a table actually
   listed in `db/OWNERSHIP.md` is reported — the identifier a keyword is
   followed by must be a real table name, not merely absent from the
   scanning module's own list, or a `slog` call like `"failed to update
   payment status"` would report a violation on every run once service and
   command files are in scope. A CTE named after a real table is its own
   violation, not an exemption — one `WITH orders AS (...)` would otherwise
   silence every genuine reference to `orders` in the file. `dashboard`
   exempt by name — reporting read-model. Change ownership in
   `db/OWNERSHIP.md`; no list in the script to keep in step.
4. **`check_cross_module_imports`: from another module, only
   `<feature>/contract` is importable.** One sentence, and it covers
   domain-privacy, slice-privacy and contract-as-published-API together —
   `domain/`, every slice's root package, and every slice's `postgres`/
   `http`/`redis` adapter are all equally off-limits to every other module.
   Exempt only as an importer: `internal/bootstrap/` and
   `internal/transport/`, the wiring layer that exists to assemble modules.
   Same-module imports are unrestricted here — a slice reaching a sibling
   slice in its own module is check 5's concern, not this one's.
5. **`check_sibling_slice_imports`: a slice imports no sibling slice within
   its own module.** Check 4 cannot see this — both packages live under the
   same module, so there is no cross-module path for it to flag. **"Slice" is
   now defined by one fact: a directory under `<feature>/usecase/`.** That
   replaced a seven-name denylist (`domain`, `contract`, `http`, `gateway`,
   `worker`, `postgres`, `redis`) which had to be kept in step by hand. Its
   grep is scoped to `<feature>/usecase/` as well, not `<feature>/` — without
   that, a slice's own `<feature>/domain` import has no `usecase/` segment to
   strip and gets reported as a sibling slice named `github.com`.
   **The new rule is not a pure superset of the old one.** `jobs` was absent
   from the denylist, so `notification/jobs` and `payment/jobs` used to be
   scanned as slice-importers and a slice importing `<feature>/jobs` was
   reportable; both now sit outside the `usecase/` walk and neither case is
   checked. No such import exists today. A slice needing a sibling's
   capability declares a port in its own `ports.go` and lets `module.go`
   supply the sibling by name-match or a `contract/` package — importing the
   sibling directly is exactly the coupling that pattern exists to prevent.
6. **`check_transport_direction`: a module may not import
   `internal/transport`, except a slice's own `http/` adapter.** That is now
   the **one** exempt location, down from two: the feature route tables that
   were the second are gone, so no file under a module names a route or takes
   a `middleware.RouteGroup`. The check catches a service returning a
   transport type, and a `module.go` that would register routes — either
   would make every binary constructing the module link HTTP, including the
   worker, which serves nothing. A module that needs to describe something
   the transport also describes puts the type in its own `contract/` and lets
   middleware import that instead (`user/contract.AccountStatus`,
   `auth/contract.Claims`).
7. **`check_contract_leaf`: `contract/` imports only stdlib,
   `github.com/google/uuid` and `internal/money`.** If a module's
   `contract/` imported its own `domain/`, importing the contract would drag
   the rich model along, and the module's published surface would silently
   become everything. Check 4 cannot see this either — a `contract/` file
   importing its own module's `domain/` is a same-module import.

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
literal can produce a loud false positive. Check 3 scans the whole module
now — a stray query outside any `postgres/` directory is no longer a blind
spot the way it was before this phase.

### Conventions — not checked, so they need you

10. **A module or a slice never imports another module's root package,
    `domain/`, or another slice's adapter — only `<feature>/contract`.**
    This is now check 4's job, sharpened to the sentence it actually
    enforces (AGENTS.md used to state a narrower version, catching only
    an adapter import; the check now catches the root-package case too).
    **The arrow runs the other way for URLs: transport imports modules, and
    a module names no URL.** Route tables live in
    `internal/transport/http/routes/`, one file per feature, 14 of them, each
    a package-level `func <Feature>(groups…, m *<feature>.Module, …)` that
    `internal/transport/http/router.go` calls once. A slice supplies a
    handler with exported route methods; the transport decides the verb, the
    path and which `middleware.RouteGroup` it lands on. `ARCHITECTURE.md`
    decision 15 is why, and its cost. Declare the interface _the consumer_
    needs in the consumer's own package — a slice's `ports.go`, or
    `module.go` when several slices share it (see "Inside a feature", above).
    Two mechanisms satisfy the port without an adapter:
    - **Name-match.** The producer's own value already has a method named
      what the consumer's port asks for. `promotion/usecase/reserve.UseCase`
      satisfies both `order.CouponPort` (`Reserve` + `Release`) and
      `payment.CouponPort` (`Release` alone) directly — two differently-shaped
      interfaces, same producer value, no adapter for either. Notification's
      `jobs.Worker` satisfies `platform/jobs.Processor` directly.
      `order/usecase/transition.UseCase` — the standalone value
      `internal/bootstrap/app.go` builds before `order.New` can run —
      satisfies `payment.OrderTransition` directly, which is what breaks the
      order/payment cycle at slice granularity. `order.Module` itself
      satisfies `shipping.OrderPorts` through its `GetInfo`/`MarkShipped`/
      `MarkDelivered` delegators, the same trick one level up.
    - **A `<feature>/contract/` package**, when what crosses is a struct
      rather than a scalar or an interface a producer already satisfies.
      The consumer's port still names the type it needs
      (`refresh.UserProvider.GetByID(ctx, id) (usercontract.User, error)`);
      the contract package only supplies the shape, never the interface.

    No shared ports package, and adding one would defeat the point.
11. **Services take `database.TxRunner`, never `*pgxpool.Pool`.** Service needs atomicity, not DB handle. `TxRunner` declared once in `internal/platform/database` not per consumer — one deliberate exception to rule 10's consumer-declaration pattern, because features already import `platform/database`. A slice that opens no transaction takes no runner at all.
12. **Money is `money.Money`, never an `int64` beside a `Currency string`.** Scope is four features: `order`, `payment`, `product`, `cart`. `promotion` and `dashboard` stay on `int64` for stated reasons — `ARCHITECTURE.md` §10 and `ARCHITECTURE-LIMITATIONS.md`. `Money` carries no `json` tag and implements no `sql.Scanner`: each adapter maps it explicit, because wire shapes genuinely differ per endpoint. No float constructor and no `Div`.
13. **A slice runs no SQL and holds no pool.** Every read and write goes through the slice's own repository interface; its `postgres/` adapter owns the pool and reaches it with `database.DB(ctx, pool)`, which returns the context's transaction if there is one. A `UseCase` composes several repository calls into one unit of work via its `TxRunner`, and the transaction propagates to every repository it touches — own and other modules' — through `ctx`.
14. **Order status changes only through `order/usecase/transition.UseCase.Apply`.** Every guarded transition is a named `domain.Transition` value in `internal/modules/order/domain/transition.go` (`PaidTransition`, `RefundTransition`, `CancelledTransition`, …). Other modules depend on _intent_ methods on their own port interface (`payment.OrderTransition.MarkPaid`, `shipping.OrderPorts.MarkShipped`/`MarkDelivered`), and each caller wires to a value that implements them — `order/usecase/transition.UseCase` directly, or `order.Module`'s delegators, which just call `Apply` with the right transition. Never write an ad-hoc from/to status list at a call site.
15. **Inventory reversal goes through `inventory.Module.Restore(ctx, items
map[uuid.UUID]int, prior contract.StockState)`.** Inventory decides whether that means releasing a reservation or restocking deducted goods; callers supply order's prior state, never the mechanics. `StockState` lives in `inventory/contract` — `order` is the caller and names the type without importing inventory's implementation.
16. **Background job workers use `platform/jobs`.** The package draining a queue implements `jobs.Queue[T]` (`Claim` + `Prune`) and `jobs.Processor[T]` (`Process`) on whichever value the binary hands to `jobs.Runner[T]`, plus an optional `jobs.Sweeper` for per-tick housekeeping. `payment/jobs.Queue` and `notification/jobs.Worker` are the two queues today; `payment/jobs.Dispatcher` is the processor that routes a claimed payment job to charge or refund, and `payment/worker.Processor` wraps the dispatcher to add order's stale-order sweep. All four live at a feature root, outside `usecase/`, and none is a slice. Never hand-roll a ticker/lease/poll loop — the runner owns polling, leased compare-and-set claim, bounded concurrency, per-job timeouts and pruning.
17. **Repository reads use `pgx.CollectRows`**, never a hand-rolled `for rows.Next()` loop. Escape search terms with `database.EscapeLike()` and build keyset predicates with `database.KeysetCursor()`.
18. **Handlers use the shared helpers.** Decode and validate with `response.Bind[T](w, r, h.validator)`; read caller with `middleware.RequireUser(w, r)`; return errors through `response.HandleErr`. Do not hand-roll decode/validate or auth-context blocks.
18a. **A slice `http/` port is named for the role it plays, never for the
    pattern.** `CartAdder`, `ProductCreator`, `Authenticator`, `OrderPlacer`,
    `WebhookProcessor` — all 53 slice `http/` packages, 56 ports, no exceptions.
    Never `UseCase`: the directory already says the value is a use case, so the
    name would add nothing, and `placehttp.UseCase` would sit one import from
    `place.UseCase`, two different things sharing a name. Three packages hold
    two ports because they split routes by caller role (`user/usecase/query/http`
    `UserGetter` + `UserLister`, `product/usecase/query/http` `ProductReader` +
    `AdminProductReader`, `order/usecase/query/http` `OrderReader` +
    `AdminReader`); role naming is what lets them coexist. The concrete type in
    the slice root is the opposite rule — always `UseCase`, never a role name.
    The `Handler` **field** holding that port is always `usecase`, and so is the
    `New` parameter that sets it: `h.usecase.Execute(...)` in every one of the 56.
    A field is private and there is only ever one, so it is named for the layer,
    not the role. Never `cmd` or `reader` — that vocabulary retired with the
    `Command`/`Reader` types.
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
- **Docker is required.** No build tags, no short mode. `internal/testhelper` starts two long-lived containers by fixed name (`go-api-test-postgres`, `go-api-test-redis`) and every test binary attaches to whichever already exists. Remove them with `make test-clean`.
- **SQL semantics stay in adapter's own test.** Recursive CTEs, keyset pagination, unique constraints — anything only DB can prove — belong in `internal/modules/<feature>/usecase/<slice>/postgres/repository_test.go` (or `redis/` adapter's own `cache_test.go`) against real container. Anything mock can express — a `UseCase`'s reaction to value, error branch — belongs in that slice's `usecase_test.go` instead, and a saga spanning tables no single feature owns goes to `test/e2e/` (below). No slice starts its own container. `go test ./...` runs package binaries concurrently; collapsing per-package tests into one `test/integration` package would make them sequential. `ARCHITECTURE.md` decision 11 rejects that directory explicit.
- **`test/e2e/` is for sagas no single feature can own** — checkout, payment, refund, fulfilment failure, admin flows — driven through real `apihttp.NewRouter`, real Postgres, and mock gateway on `httptest.Server`.
- **Postgres databases are per module, created once under an advisory lock, and never dropped; Redis indices are still a slot you claim.** `MustStartPostgres(dbName)` creates and migrates `dbName` the first time any caller asks for it — the lock covers the migration too, so a second caller that finds the database already there also finds it already at the latest schema — and every later caller, same test binary or a different one, just connects. Every slice's test package under one module passes the same name on purpose — `test_shipping`, `test_order`, `test_payment`, and so on for all fourteen (`grep -rn 'MustStartPostgres(' --include='*_test.go' internal/modules` shows the current mapping). Packages sharing a name no longer tear each other down, but they also get no clean table between them: seed the rows your subtest asserts on and never `TRUNCATE`. **`ResetDB` is now safe only for a package that owns its database outright — nothing inside `internal/modules` qualifies any more, now that every module is sliced.** Today's three callers are `internal/bootstrap/app_test.go`, `internal/transport/http/router_test.go` and `test/e2e/testmain_test.go`, each with its own private database. `ResetDB` takes a `*pgxpool.Pool`, not a package name, so nothing stops a fourth caller inside a module from adding it — check before copying a `setup` helper that calls it. `MustStartRedis(dbIndex)` takes an index a caller picks by hand, against the registry comment above that function in `internal/testhelper/testhelper.go`. Indices 0, 1, 3, 5 and 6 are claimed (`platform/cache`, `transport/http/middleware`, `transport/http`, `test/e2e`, `user/usecase/query/redis`); 2 and 4 are free. Nothing enforces a claim — collision compiles, passes review, fails as flake in unrelated package — so update that comment in the same commit that takes an index, and cross-check it against `grep -rn 'MustStartRedis(' --include='*_test.go' .`, which is the only record that cannot drift.
- **`t.Parallel()` buys nothing in a package that owns a database or a Redis
  index**, because everything in that package shares one connection and `ResetDB` TRUNCATEs every table in it. Those packages excluded from `paralleltest` wholesale in `.golangci.yml` -- per package, never per file, because parallel sibling gets its rows deleted mid-assertion even when that sibling never calls reset itself. Nothing given up: `go test` already runs packages concurrently and each owns own database. Have each subtest seed own data instead.
- **Everywhere else `t.Parallel()` is mandatory**, and `paralleltest` enforces it on both test function and every `t.Run` closure. If you add test package claiming database or Redis slot, add it to that exclusion list in same commit.
- **Order a test file so the tests come first.** Package-level `var`s and `TestMain` at top, then every `func TestXxx`, then stub types with their own methods grouped under them, then plain helpers last. `internal/platform/jobs/runner_test.go` is the shape. Someone opening file came for scenarios, not fakes that serve them. `funcorder` only orders methods against their struct, so nothing lints the rest — on you.
- **Prefer subtests over table-driven tests.** One logical scenario per subtest, descriptive name, own setup. Break large scenarios up; no monolithic tests.
- **Compare whole objects, not field by field.** `assert.Equal(t, expected,
result)` on full struct or slice. For JSONB round-trips use `assert.JSONEq` — Postgres normalises whitespace.
- **Test behaviour, not wiring.** Verify returned value, error, or side effect.
- **Mocks are generated** by mockery v3 from `.mockery.yml` **in-package**, as `mocks_test.go` beside the interface they mock. A `_test.go` file never enters its package's importable `GoFiles`, so a mock is private to that package and cannot cycle back — which lets a slice's own `usecase_test.go` stay `package <slice>` and its `handler_test.go` stay `package http`, each mocking only the interfaces declared in its own package, and keeps every `Mock*` name out of the module's exported API. `.mockery.yml` is one recursive rule rooted at `internal/`, `all: true`, no per-interface `configs:` list — every interface gets exactly one mock, in its own package. This is why a handler declares its own narrow interface locally rather than importing one from its slice's root package: mockery cannot write a private mock into a package that does not declare the interface, so an interface consumed across a package boundary would need it declared on the consumer's side either way, and every slice does. Run `make mocks`; never hand-edit the generated file. Use the expecter API (`repo.EXPECT().GetByID(mock.Anything, id).Return(...)`), never `repo.On("GetByID", ...)`.
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
- Do not suppress lint or vet findings with `//nolint` without justification comment on same line — see `order/usecase/place/usecase.go:68` (`//nolint:gocognit,funlen // checkout orchestrates idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction`) for expected form. Four more slices carry the same pattern for the same reason: `order/usecase/cancel/usecase.go`, `payment/usecase/webhook/usecase.go`, `payment/usecase/charge/usecase.go`, `payment/usecase/refund/usecase.go`.
- Do not make subpackage tree uniform, and do not add pass-through adapter package to fill slot.
- Backward compatibility explicitly **not** a goal here. API shapes may change where better design demands — but say so when they do.
- When adding a feature: create `internal/modules/<feature>/` with `domain/` for its aggregate, one `module.go` per the shape under "Inside a feature" above, and `usecase/<slice>/` per use case, each with `usecase.go` declaring one `UseCase`, `repository.go`, `postgres/`, and `http/` where it has a route. Add a row per owned table to `db/OWNERSHIP.md`, add `internal/transport/http/routes/<feature>.go` and call it from `internal/transport/http/router.go`, and wire the module into `internal/bootstrap/app.go` — by name-match if an existing port already fits, or by adding a `contract/` package if a struct needs to cross. **Adding one slice with one route touches two trees**: the module for the handler, `routes/<feature>.go` for the URL. Then run `make check-boundaries` — a new feature with a `postgres/` adapter and no ownership row fails it by design.

## Further reading

- `README.md` — endpoint reference and quick start. Its "Project Structure" section agrees with this file; both rewritten against real tree. Its environment table is **curated subset** — 8 variables absent, including the whole Redis pool group (`REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`, `REDIS_DIAL_TIMEOUT`, `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT`, `REDIS_POOL_TIMEOUT`) and the worker's prune settings (`WORKER_PRUNE_AGE`, `WORKER_PRUNE_LIMIT`). `.env.example` is the exhaustive list; verified against `envconfig` tags across `internal/platform/config/config.go` (infra) plus each module's own `config.go` (`auth`, `cart`, `order`, `payment` — the four with env vars of their own).
- `ARCHITECTURE.md`, `ARCHITECTURE-LIMITATIONS.md`, `db/OWNERSHIP.md` — as above.
- `db/migrations/` — goose SQL migrations.
- `.env.example`, `.mockery.yml`, `.golangci.yml`.
