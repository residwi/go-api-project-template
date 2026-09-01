# Architecture

Why this codebase is shaped the way it is, what each decision costs, and what
the shape makes hard. One line per decision; the code and `AGENTS.md` carry
the rules themselves.

**The numbering is stable.** `scripts/check-boundaries.sh`, `db/OWNERSHIP.md`
and `AGENTS.md` cite these sections by number, so a retired or reversed
decision keeps its number instead of being renumbered away.

## Decisions

**0. This repository is a template, so the structure is the product.** Every
decision below is judged by what it teaches, not by what is cheapest to
maintain: a reader will copy whatever they find here into a real system, so
where a rule exists it is machine-checked. *Cost:* the `adapter/postgres` /
`adapter/http` split buys an import alias per module in `bootstrap/app.go` and
`server/router.go`, which is hard to justify in a product codebase and is the
point here. Backward compatibility is explicitly not a goal.

**1. Feature modules, not layers.** A module owns its whole vertical — domain,
service, storage port, adapters — so a feature changes in one directory instead
of four. *Cost:* a module is a directory tree, not a file.

**2. Ports live with the consumer.** The consuming module declares the
interface it needs in its own `ports.go`; the producer never publishes one.
*Cost:* free where a producer's method already matches by name; where a struct
has to cross, decision 13 pays for it with a published surface.

**3. Adapters are subpackages named for their technology.** `adapter/postgres`,
`adapter/http`, `adapter/redis`, `adapter/gateway`, `adapter/jobs`. *Cost:*
many packages share a name across modules, so wiring files alias them.

**4. Adapter subpackages exist only where adaptation is needed.** No
pass-through package to fill a slot. *Cost:* you cannot predict a module's
shape without looking.

**5. Services take `database.TxRunner`, never `*pgxpool.Pool`.** A service
needs atomicity, not a database handle. *Cost:* one interface with one
production implementation forever — textbook YAGNI, accepted because it lets
the compiler police the narrower type. It does not make transactions explicit;
the transaction still travels ambiently in `context`.

**6. Modules own their data.** A module's SQL names only tables it owns, and
`db/OWNERSHIP.md` is the parsed record. Cross-module reads go through a port.
*Cost:* two queries where one join would do, and `?in_stock=true` becomes
unimplementable. `dashboard` is carved out as a reporting read model.

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
error. Check 4 makes a module's root package importable, so `payment` *can*
call `order.Place`, and no check can tell that from a legal import.

**17. Ports collapse to one per producer, not one per capability.** A consumer
declares one interface per module it consumes, holding every method it needs.
*Cost:* the port stops answering "what does this specific call path use" —
`order.Cart` carries `Lock`, `Snapshot` and `Clear` whatever the caller needs.

**18. Background jobs share one queue, not one per module — and that queue is
River.** `internal/platform/queue` holds an insert-only client and a
transaction-aware `Insert`; each module keeps its args, `InsertOpts` and
`river.Worker` in its own `adapter/jobs`, and `internal/worker` owns the one
working client. *Cost:* `river_job` has no foreign key to any module's rows, so
the old per-module job tables' referential integrity is gone for good.

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
- OpenTelemetry wiring — nothing here exports traces yet.

## Limitations

What this shape makes hard or impossible. Read the relevant entry before
proposing a feature that crosses a module boundary.

### Boundaries and coupling

- **A module's whole exported surface is reachable from every other module.** Check 4 allows a root-package import, so a sibling's method is one call away and no check can distinguish it from a legal one. The port convention is all that stands there.
- **`checkout` is held to a weaker rule than its siblings** — it alone may import a module's `domain/`, because `order.Service.Place`'s signature is written in `orderdomain` types.
- **One flat `Service` satisfying several of a consumer's ports leaves the compiler unable to check which value goes where.** Two slice values used to be two distinct types; one `Service` satisfying both is one type, so wiring the wrong value would still compile. Nothing misbehaves today because every such field is wired to that same value.
- **`contract.go` can grow into the shared domain model `internal/shared/` was rejected for being.** Nothing bounds what a module publishes there.
- **Extracting a module into a service is a data migration, not a refactor** — the foreign keys decision 8 keeps are what make it one.
- **Foreign-key fan-in is not the dependency graph.** A table many others reference is not therefore a module many others depend on, and reading the schema for coupling misleads.

### What the checks cannot see

- **`make check-boundaries` has blind spots, and they are where you would hide something:** a table name must be a string literal, `_test.go` files are skipped, `dashboard` is exempt wholesale, and every check walks `internal/` only — `cmd/` and `test/` are outside all of them.
- **A module can still smuggle SQL past check 3 with a built string.** The check is a grep, not a compiler.
- **A path-keyed check can quietly stop matching anything.** `scripts/boundaries_test.go` probes each check from both sides for exactly this reason; the `paralleltest` exclusions in `.golangci.yml` have no such probe.
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

- **Job delivery is at-least-once, and only one of the two jobs is idempotent.** A redelivery of the other one repeats its effect.
- **`RescueStuckJobsAfter` is client-wide**, so a per-queue rescue window is not expressible — only a per-worker `Timeout()` is.
- **The read replica is wired up with no protection against reading your own write.** `ReplicaDB` is a per-method choice, and nothing checks whether that method follows a write.
- **A repository write can leak outside its own transaction with no test failing** — the transaction travels in `ctx`, so a method handed the wrong context still runs.

### Tests and wiring

- **The test suite shares one Postgres and one Redis, and Redis slots are hand-assigned.** A collision compiles, passes review, and fails as a flake in an unrelated package.
- **The composition site is deliberately tedious.** `bootstrap.New` is one long list of positional constructor arguments, which is what makes a forgotten dependency a compile error.

## When not to copy this

This tree pays for boundaries a script can enforce because it is a template
and the structure is the lesson. A product codebase with one team, one
deployable and no copies downstream will find several of these decisions —
the adapter package split, the consumer-declared ports, the per-module table
ownership — cost more than they return. Take the checks and the ownership
document first; they are the parts that keep working when nobody is watching.
