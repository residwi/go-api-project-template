# Architecture

Why this codebase is shaped the way it is, what each decision costs, and what
the shape makes hard. One line per decision; the code and `AGENTS.md` carry
the rules themselves.

**The numbering is stable.** `AGENTS.md` cites these sections by number, so a
retired or reversed decision keeps its number instead of being renumbered away.

## Decisions

**0. This repository is a template, so the structure is the product.** Every
decision below is judged by what it teaches, not by what is cheapest to
maintain: a reader will copy whatever they find here into a real system, so
a rule is enforced by go-arch-lint where a linter can see it, and recorded as
a convention where it cannot. *Cost:* the `adapter/postgres` /
`adapter/http` split buys an import alias per module in `app/app.go` and
`server/router.go`, which is hard to justify in a product codebase and is the
point here. Backward compatibility is explicitly not a goal.

**1. A modular monolith of hexagons, not a layered application.** One
deployable and one database, but a module owns its whole vertical — domain,
service, the ports it declares, the adapters that satisfy them — so a feature
changes in one directory instead of four, and the hexagon is per feature
rather than one for the whole application. *Cost:* a module is a directory
tree, not a file.

**2. Ports live with the consumer.** The consuming module declares the
interface it needs in its own `ports.go`; the producer never publishes one.
*Cost:* free where a producer's method already matches by name; where a struct
has to cross, decision 13 pays for it with a published surface.

**3. Adapters are subpackages named for their technology, and each file is
named for the port it satisfies.** `adapter/postgres/repository.go`,
`adapter/jwt/tokens.go`, `adapter/gateway/stripe/gateway.go`.
*Cost:* many packages share a name across modules, so wiring files alias them.

**4. Adapter subpackages exist only where adaptation is needed.** No
pass-through package to fill a slot. *Cost:* you cannot predict a module's
shape without looking.

**5. Services take `database.TxRunner`, never `*pgxpool.Pool`.** A service
needs atomicity, not a database handle. *Cost:* one interface with one
production implementation forever — textbook YAGNI, accepted because it lets
the compiler police the narrower type. It does not make transactions explicit;
the transaction still travels ambiently in `context`.

**6. Modules own their data.** A module's SQL names only tables it owns, and
cross-module reads go through a port. *Cost:* two queries where one join would
do, and `?in_stock=true` becomes unimplementable. `dashboard` is carved out as
a reporting read model. Nothing enforces this — a module's SQL may name
another module's table and every check will pass.

**7. Inventory owns stock; product does not.** *Cost:* creating a sellable
product is two admin calls. `available_stock` is stored, not derived, so every
operation keeps it correct.

**8. Foreign keys stay; cross-module cascades do not.** In a single database,
referential integrity Postgres enforces beats discipline code review enforces —
but a cascade that can never fire (both `users` and `products` are
soft-deleted) is a lie in the schema, and a lie is worse than an absence.
*Cost:* the dropped cascades imply cleanup that no longer happens anywhere.

**9. `adapter/http` owns the wire format.** No `json` tag outside it, no
`json:"-"` anywhere, no `dto.go` at all — adding a field to a response means
naming it in a wire type deliberately. *Cost:* many mapper functions, request
types split into a core `…Params` plus an unexported wire type, and the failure
mode that replaced `json:"-"` is naming the *wrong* mapper: `toUserResponse`
and `toAdminUserResponse` sit in one package and either compiles.

**10. `money.Money`, not an `int64` beside a `Currency string`.** Scope is
`order`, `payment`, `product`, `cart`. *Cost:* explicit two-column mapping in
every `postgres` adapter and flattening in every response type, because wire
shapes genuinely differ per endpoint — `cart`'s `total` carries no sibling
currency while its items carry both. `promotion` and `dashboard` stay on
`int64`.

**11. Integration tests stay next to their code; only e2e is centralised.**
SQL semantics belong in the adapter's own test against a real container.
*Cost:* every claiming package needs its own `TestMain` and its own database
name.

**12. Log attributes travel in the context, not in signatures.** A service
that logs `request_id` has no business knowing what an HTTP request is, so
`logger.WithAttrs` stores attributes and `logger.ContextHandler` merges them
into every record below. *Cost:* the attributes are write-only — nothing can
read back what the context carries.

**13. `contract.go` publishes the structs that cross a boundary.** A module
earns one only when a struct — not a scalar, not something a producer already
satisfies by name — has to cross a port. *Cost:* a published surface: changing
a contract type is a change every consumer absorbs. The old `contract/` package
was additionally proven leaf-import-clean by a check; `contract.go` in the root
package cannot be, and that guarantee is gone.

**14. REVERSED — a module is a business boundary containing vertical slices.**
Kept as history: this is why the tree held 226 packages of `usecase/` slices for
a year. Decision 16 records what replaced it and what the reversal cost.

**15. The transport owns every URL; a module owns none.** Every route lives in
`internal/server/router.go`; a module supplies a handler with exported route
methods. *Cost:* real. Adding a route touches two trees, they can be edited
apart — a handler method no route mounts compiles clean and serves nothing —
and a module is no longer copy-pasteable with its routes.

**16. A module is one flat package with an `adapter/` directory.** One
`Service` per module, no `usecase/`, no `Deps` struct, no `module.go`. *Cost:*
read this before copying the shape — module privacy stopped being a compile
error. A module's root package is importable from anywhere, so `payment`
*can* call `order.Place`, and nothing distinguishes that from a legal import.

**17. Ports collapse to one per producer, not one per capability.** A consumer
declares one interface per module it consumes, holding every method it needs.
*Cost:* the port stops answering "what does this specific call path use" —
`order.Cart` carries `Lock`, `Snapshot` and `Clear` whatever the caller needs.
Segregation comes from the published types instead: `order` offers `Snapshot`
and `FulfilmentSnapshot` so a consumer names the one whose fields it drives.

**18. Background jobs share one queue, not one per module — and that queue is
River.** `internal/platform/jobqueue` holds an insert-only client and a
transaction-aware `Insert`; each module keeps its args, `InsertOpts` and
`river.Worker` in its own `adapter/jobs`, and `internal/worker` owns the one
working client. *Cost:* `river_job` has no foreign key to any module's rows, so
the old per-module job tables' referential integrity is gone for good.

**19. Authentication is platform middleware over a feature port.**
`platform/web/middleware.Auth` reads the bearer header and calls an
`Authenticator`; `auth.Service.Authenticate` decides token kind, account status
and revocation; the `identity.Identity` it returns lives in `platform/identity`
so a service can produce one without importing the transport ring. The JWT
library sits behind `auth.Tokens` in `adapter/jwt`, and `Verify` takes the kind
the caller expects rather than returning it, so no caller can forget to check.
*Cost:* `platform` now defines an interface a feature must satisfy and an
identity shaped like this application's — `UserID` and `Role` — which a project
with scopes or non-UUID subjects would edit rather than copy. `Require(pred)`
absorbs the first of those without touching the type.

**20. Edges are instrumented by library; business spans are hand-written.**
`otelhttp`, `otelpgx`, `redisotel` and `otelriver` wrap every port this
template does not own, so a route, a query, a Redis command and a job already
produce a span with no code in the module that uses them. A `Service` method
has no library to wrap the same way — the only automatic option is a
decorator, which needs one interface per `Service`, the shape decision 16
spent itself avoiding — so the three-line prologue `AGENTS.md` names is
written by hand instead, once per exported `Service` method that takes `ctx`.
*Cost:* a near-identical block per method, each a place to get the ordering
wrong; `AGENTS.md` states the ordering rule so at least the mistake is
checkable by eye.

**21. The cache seam is a `Repository` decorator.** Policy — barrier, jitter,
not-found placeholder, corrupt-entry drop — lives in `internal/platform/cache`;
key naming and encoding live in the feature's `adapter/redis`; the `Service`
is untouched and arch-lint forbids it importing either. *Cost:* the port does
not say which methods are cached, so a reader must open the decorator. Each
decorator writes every port method out rather than embedding, so a new method
is a compile error instead of a silently uncached read. Invalidation is a
`Del` fired after the write commits, not a lock held across it, so a read
already in flight can load the pre-write row and `Set` it back after the
`Del` runs, leaving that stale value cached for a full TTL — only
compare-and-set closes that window, and neither `ardanlabs/service` nor
go-zero closes it either, for the same reason.

**22. Five read paths, three mechanisms, chosen per call site.** `ReadThrough`
where the barrier or the absent placeholder pays (`category.List`,
`product.GetBySlug`, `promotion.GetByCode`); a maintained atomic counter
where the writes already know the delta (`notification.CountUnread`,
decision 23); plain cache-aside where neither applies
(`product.GetImagesByProductID`). *Cost:* three patterns instead of one, and
the cache-aside site is twenty lines where `Take` would have been three.
`product/adapter/redis` holds two of the three so the difference is readable
in one file. `GetImagesByProductID` is reached through two call paths and
only one is barriered: the public `GetBySlug` and the admin-only `GetByID`
(`product/service.go:226`, behind `GET /api/admin/products/{id}`) both call
it, and `GetByID` has no barriered read in front of it. Harmless today —
admin-only, low request volume — but a reader should not assume every call
into it is barriered.
`dashboard` is not cached at all. All four repository methods already read
through `database.ReplicaDB`
(`dashboard/adapter/postgres/repository.go:26,44,63,88`), so these aggregates
never compete with the transactional path once a replica is configured. A
decorator here would pay to protect a query that, on that topology, never
reaches the primary in the first place, and it would cost more than it
returns: a staleness window on admin analytics, and a silent failure mode
where a wrong report key yields a zero-valued struct with a nil error,
indistinguishable from "no results in this period" to the caller. All three dashboard routes
also sit behind `RequireRole("admin")`, so there was never meaningful read
pressure to protect against either. *Caveat:* `REPLICA_DATABASE_URL` defaults
to `""` (`internal/config/config.go:93`) and `ReplicaDB` falls back to
`Primary` when no replica is configured, so in a single-database deployment
these aggregates do hit the primary — but the fix for that is a replica or
an index, not a five-minute cache, which only helps a repeated identical
date range and does nothing for an admin sweeping different ranges.

**23. The unread counter is one atomic Lua `EVAL`, not `INCR`/`DECR`.**
`notification/adapter/redis/repository.go`'s `adjustScript` runs `EXISTS`
then `INCRBY` then, if the result went negative, a floor to zero — all in one
round trip. Two round trips would race: `INCRBY` on a missing key
auto-vivifies it and sets no TTL, so a check-then-increment whose key expires
between the `EXISTS` and the `INCRBY` leaves a counter stuck at the delta
forever — `GET` keeps succeeding, never returns `redis.Nil`, and the recount
that was meant to bound the drift never runs. The script's protective
property is that it never creates the key on a miss; `adjust()` calls
`.Eval(...).Err()` and discards the result, so nothing on the Go side
consumes the value the script returns. The reload instead comes from
`CountUnread`'s own independent `GET` plus its `redis.Nil` check — the
script does return `-1` on the absent path today, but no caller reads it.
*Cost:* the script is a string literal with no compiler behind it, and it is
the one place in the tree that names Lua.

**24. Never serve a cached read inside a transaction.** `database.InTx`,
applied at one call site — `promotion.GetByCode`, because `Reserve` prices an
order from the row it reads. *Cost:* the rule is enforced by review, not by a
linter, and the guard is absent from the other three decorators because no
transaction reaches them today. The tripwire is
`product.GetByIDsIncludingDeleted`: `order.Place` reaches it through
`cart.Snapshot` inside its own transaction, so caching it without the guard
would write a stale price into a placed order.

**25. `user.GetProfile` is not cached.** `auth.Authenticate` reads it on
every authenticated request, which makes it the highest-fan-in read in the
tree — and the freshness of that read is what makes `token_version` mean
anything. Caching it substitutes a TTL for a revocation mechanism. gitea has
the identical structure (`services/auth/session.go:43` reads the user row
uncached on every request even when sessions live in Redis, and it has no
logout-everywhere); hydra ships a stateless JWT introspector and refuses to
wire it for the same reason; zitadel's cache `Purpose` enum contains no user,
session or token entry. *Cost:* one `SELECT` per authenticated request,
permanently.

**26. `readthrough.go` falls through to Postgres on a Redis read error**,
where go-zero fails fast (`core/stores/cache/cachenode.go` at commit
`84c92d7`, "we don't allow the disaster pass to the dbs"). This repository
protects availability; go-zero protects the database. *Cost:* a Redis outage
sends full read traffic at Postgres.

**27. Exactly one `//nolint:musttag` exists in the tree**, at
`product/adapter/redis/repository.go:101`, and `nolintlint` runs with
`allow-unused: false` so a clean lint proves it is still consumed. It is
needed only where `json.Unmarshal` targets a concrete domain type —
`[]domain.Image`, there. Every call inside `cache.ReadThrough.fill[T any]`
needs no suppression, because `musttag` cannot resolve an unmarshal target
through a type parameter. That is an accident of the analysis, not a
safety property earned by the generic path — the rule this leaves is that
domain types carry no `json` tags (decision 9), so a json-tagged mirror
struct in `adapter/redis` would violate that harder rule *and* silently drop
any field added to the domain type later without one. A scoped suppression
with a same-line justification is therefore the right tool exactly where a
concrete type meets `json.Unmarshal`, and nowhere else. *Cost:* the reasoning
is invisible to a reader who does not already know `musttag` cannot see
through a type parameter, so the next concrete-type unmarshal in a new
adapter will need this comment written again from scratch rather than found
by example.

## Foreign keys across module boundaries

22 foreign keys exist and 16 cross a module boundary. All 16 stay. The 6 that
do not cross are aggregate-internal and unremarkable: `cart_items→carts`,
`categories→categories`, `coupon_usages→promotions`, `order_items→orders`,
`product_images→products`, `wishlist_items→wishlists`.

**Inbound foreign keys are not the dependency graph.** `users` has 6 inbound
FKs and 1 inbound port; `products` has 6 and 2; `categories` has 2 and none.
Reading the schema for coupling misleads, which is why extracting a module is
a data migration rather than a refactor.

**Cross-module `ON DELETE CASCADE` is not kept.** Migration
`20260424120016_drop_cross_module_cascades.sql` dropped five cascades while
keeping each reference: `carts.user_id`, `cart_items.product_id`,
`wishlists.user_id`, `wishlist_items.product_id`, `notifications.user_id`.
They were unreachable — `users` and `products` are soft-deleted, so no
`DELETE` ever reaches those rows — and meanwhile the schema advertised a
cleanup nothing performed and described the database writing across a module
boundary that no port describes. A lie in the schema is worse than an absence.

## Deliberately not done

Each of these was considered and rejected; the reason is the same shape every
time — it would either cross a module boundary or add a layer that buys
nothing here.

- `internal/shared/` — a shared domain model is the thing module ownership exists to prevent.
- Typed IDs (`ProductID`, `UserID`) — every id already travels as `uuid.UUID` through a typed port.
- `shared/address`, `shared/events` / a domain event bus — one shared struct or bus re-couples every module that touches it.
- Multi-warehouse inventory — a redesign, not a column.
- `x/grpc/`, `x/webhook/` as a package, `notification/worker/` — speculative surface with one caller.
- `platform/uuid`, `platform/clock` — wrappers over stdlib with nothing to swap.
- `test/integration/` — would serialise tests `go test ./...` already runs concurrently.
- A `product_view` read model — `dashboard`'s carve-out already covers reporting.
- Backward compatibility — see decision 0.
- A logger in the context — decision 12 puts attributes there, not the logger.
- `otelslog` / the OpenTelemetry logs signal — a different signal from tracing: it exports log records through the logs pipeline and writes nothing to stdout, so keeping the JSON logs would need a `LoggerProvider`, a log exporter and a fan-out handler. `ContextHandler` already stamps `trace_id`, and the two compose rather than compete.

## Limitations

What this shape makes hard or impossible. Read the relevant entry before
proposing a feature that crosses a module boundary.

### Boundaries and coupling

- **A sibling's tree is closed, but its root package is wide open once an edge exists.** `mayDependOn` grants a whole package, not a symbol, so declaring that `payment` consumes `order` puts every exported method of `order` within reach — the port convention is what keeps the call site honest.
- **One flat `Service` satisfying several of a consumer's ports leaves the compiler unable to check which value goes where.** Two slice values used to be two distinct types; one `Service` satisfying both is one type, so wiring the wrong value would still compile. Nothing misbehaves today because every such field is wired to that same value.
- **`contract.go` can grow into the shared domain model `internal/shared/` was rejected for being.** Nothing bounds what a module publishes there.
- **Extracting a module into a service is a data migration, not a refactor** — the foreign keys decision 8 keeps are what make it one.
- **Foreign-key fan-in is not the dependency graph.** A table many others reference is not therefore a module many others depend on, and reading the schema for coupling misleads.

### What the checks cannot see

- **`make check-arch` reads the import graph and nothing else.** It cannot see a `json` tag in the wrong package, a file named `dto.go`, SQL naming another module's table, or a module calling a sibling method no port of its own declares.
- **It cannot see which method a module calls.** Cross-module imports are declared edge by edge, so reaching a sibling's `domain/` or an undeclared sibling entirely fails the build — but any exported method of a module already on the list is invisible to it.
- **`_test.go` files are excluded**, so a test may import anything.
- **Nothing enforces the span prologue.** `AGENTS.md` names the three-line shape, but no linter checks that an exported `Service` method opening `ctx` actually starts one — a new method without it compiles clean, lints clean, and stays invisible until someone reads a trace and finds the gap.
- **A path-keyed rule can quietly stop matching anything.** go-arch-lint refuses to load when a component's glob names no directory, which covers the config itself; the `paralleltest` exclusions in `.golangci.yml` have no such guard.
- **The copy property `internal/platform` is checked for holds for `go build`, not `go test`** — four platform test packages import `internal/testutil`, which does not travel with a copied `platform`.

### Transport and exposure

- **The public and admin response mappers are one wrong import away from each other.** They live in separate files in the same package, and a handler can call either and compile.
- **Nothing tests the middleware chain `NewRouter` builds, or either rate limiter.** `TestRouteAccess` proves auth class per route, not order or behaviour of the chain around it.
- **`TestRouteAccess` only probes what `allRoutes` lists.** `web.Router` records nothing, so a mounted route missing from that hand-written table is never probed at all.
- **A sentinel's HTTP status is fixed where the sentinel is declared.** `response.HandleErr` matches only the five `errs` kinds, so a sentinel wrapping the wrong kind is the wrong status everywhere at once.

### Domain

- **You cannot filter or sort a product listing by stock** — the consequence of decision 6 named in advance.
- **Multi-warehouse is a redesign, not a column.**
- **Two queries where one join would do**, in every cross-module read.
- **Creating a sellable product takes two admin calls.**
- **The cart is not a quote:** prices are read at checkout, not frozen when added.
- **An unsellable cart line is shown, not hidden**, and **a mixed-currency cart is a 400 from `GET /api/cart`.**
- **`promotion` and `dashboard` amounts are plain `int64`**, so `money.Money`'s guarantees stop at their boundary.
- **A duplicate product id in a stock-adjustment map is silently dropped, not summed.**
- **Every keyset cursor depends on one unenforced date layout agreeing with itself** across encode and decode.

### Jobs and data

- **Job delivery is at-least-once, and one of the three workers is not idempotent.** `payment.refund` is guarded by the payment's own status and `order.expire-stale` re-derives its work set, so a redelivery of either is a no-op; `notification.send` reads the row and hands it to the channel with no dedup, so a redelivery sends the message twice.
- **`RescueStuckJobsAfter` is client-wide**, so a per-queue rescue window is not expressible — only a per-worker `Timeout()` is.
- **The read replica is wired up with no protection against reading your own write.** `ReplicaDB` is a per-method choice, and nothing checks whether that method follows a write.
- **A repository write can leak outside its own transaction with no test failing** — the transaction travels in `ctx`, so a method handed the wrong context still runs.
- **`cmd/worker` is a fourth bypass of the cache decorator.** Direct SQL — `make seed`, a goose migration, a manual `psql` session — already writes rows no decorator ever sees, since none of them call a `Service`. `internal/worker/worker.go:70` passes `nil` for the cache to `app.New`, which is the same bypass from a running Go binary: it disables invalidation for the whole process, not just caching. No job writes a cached row today — traced across all three workers, `notification.SendWorker`, `payment.RefundWorker`, `order.ExpireStaleWorker` — so there is no live bug. But `order.Place` already calls `notifications.Create` from inside a request; the day a *job* creates a notification instead, `notification.CountUnread`'s cached count silently undercounts until the TTL expires — a wrong number, not staleness, and the `GET` keeps succeeding, so nothing surfaces it. A job that writes a cached row needs the cache wired into `cmd/worker`, or its module's invalidation moved to where that write actually happens.

### Tests and wiring

- **The test suite shares one Postgres and one Redis, and Redis slots are hand-assigned.** A collision compiles, passes review, and fails as a flake in an unrelated package.
- **The composition site is deliberately tedious.** `app.New` is one long list of positional constructor arguments, which is what makes a forgotten dependency a compile error.

## When not to copy this

This tree pays for boundaries a linter can enforce because it is a template
and the structure is the lesson. A product codebase with one team, one
deployable and no copies downstream will find several of these decisions —
the adapter package split, the consumer-declared ports, the per-module table
ownership — cost more than they return. Take the layer rules in
`.go-arch-lint.yml` first: they need no per-project data file to maintain,
and they keep working when nobody is watching.
