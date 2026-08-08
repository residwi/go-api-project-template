# Architecture decisions

Why this codebase shaped this way — including things it deliberately
does **not** do, because structure only teachable if roads not taken
visible.

Read `./ARCHITECTURE-LIMITATIONS.md` for bills these decisions
carry, and `./db/OWNERSHIP.md` for table ownership map — which
`make check-boundaries` parses, so enforced not merely asserted.

---

## 0. This repository is a template, so the structure is the product

Every decision below judged by **what it teaches**, not by what cheapest to
maintain. Unusual, and changes calculus: where product codebase
would keep working shortcut, this one pays for boundary compiler can
enforce, because reader will copy whatever they find here into real system.

Two consequences worth naming, since they look like mistakes otherwise:

- `postgres`/`http` subpackage split costs ~15 aliased imports in the
  composition root (`internal/bootstrap/app.go`) and another ~15 in the
  router (`internal/transport/http/router.go`). In product, hard to justify.
  Here it the point: physical boundary teaches port/adapter distinction in way
  file naming convention cannot.
- Where rule exists, machine-checked (`make check-boundaries`). Rule
  living only in README rots, and template shipping rotted rule
  teaches rot.

Backward compatibility explicitly **not** goal. API shapes changed
freely where better design demanded.

---

## 1. Feature modules, not layers

`internal/modules/order/` holds order's domain types, service, repository
interface, and ports it needs — not `internal/domain/order` plus
`internal/application/order` plus `internal/infrastructure/order`.

**Why:** `payment.NewService()` and `payment.Repository` read naturally in Go;
`application.NewService()` and `domain.Repository` put layer name in every
import and tell nothing about what code for. Layered trees also scatter
one change across three directories.

**Cost accepted:** feature package bigger than any single layer file.

**Why the `modules/` wrapper:** 14 feature packages sit under
`internal/modules/`, one directory below old home, so
`scripts/check-boundaries.sh` reads feature list straight off
filesystem instead of maintaining denylist of everything under `internal/`
that _not_ feature. Denylist it replaced already drifted once —
`money` missing from it, so shared value object briefly treated
as module subject to ownership checks. Directory right by
construction cannot drift way list of exceptions can.

## 2. Ports live with the consumer

`internal/modules/order/ports.go` declares `InventoryReserver` — interface
_order_ needs. `inventory` does not publish it; `order` names exactly what it
needs and something else satisfies it. Module with single dependency names
file after it instead (`internal/modules/product/inventory.go`,
`internal/modules/category/product.go`); `order` has seven, so grouped.

**Why:** no module imports another, so dependency graph has no cycles by
construction and each module's port list exactly API it would need if
extracted. Pays off immediately: because interfaces declared
narrow at consumer, `promotion.Service` satisfies `payment.CouponReleaser`
directly and `notification.Service` satisfies `jobs.Processor` directly — two
adapters never needed writing.

**Cost accepted:** none, where a producer's own method already matches what
the consumer's port asks for — that is free to declare. Where what crosses is
a struct rather than something a service already satisfies by name, decision
13 (`contract/` packages) is what pays for it, and what it pays is not what
this decision originally charged. The old cost — structurally-identical types
declared in two places, plus a mapping adapter where shapes differ — is gone;
`contract/` replaced it with a published surface, and a published surface
costs something different: adding a field to it is now a cross-module change,
not a local one.

## 3. Adapters are subpackages named for their technology

`internal/modules/payment/postgres`, `internal/modules/payment/http`,
`internal/modules/payment/stripe`, `internal/modules/payment/midtrans`,
`internal/modules/payment/worker`.

**Why:** dependency rule becomes compile error not convention —
`payment` cannot import `payment/postgres` without cycle, so SQL physically
cannot leak into core.

**Cost accepted:** 13 packages named `postgres` and 14 named `http` (15 if you
count `transport/http`), so every composition site needs import aliases
(`paymentpg`, `paymenthttp`). Cost concentrates in one file, deliberately.

## 4. Adapter subpackages exist only where adaptation is needed

`payment/` has six _adapter_ subpackages (`postgres http midtrans mock stripe
worker`); `wishlist/` has two. `notification` has **no** `worker/` package
because its `Service` satisfies `jobs.Processor` directly. `contract/` is not
counted here — it adapts no technology, decision 13 covers it on its own
terms, and a module gets one independently of how many adapters it needs:
`payment/` has both six adapters and a `contract/`, `auth/` has one adapter
and a `contract/`, `wishlist/` has two adapters and no `contract/` at all.

**Why:** pass-through package created to make trees look uniform teaches that
adapters are bureaucracy. Absence is lesson.

**Cost accepted:** cannot predict module's shape without looking.

## 5. Services take `database.TxRunner`, never `*pgxpool.Pool`

**Why:** pool only ever passed to `database.WithTx` — zero direct
queries. So it _atomicity_ dependency wearing database type, 100× wider
than need, and nothing stopped service adding `s.pool.Query(...)`. It
also forced `WithTestTx` helper and ~20 `noopDBTX{}` stubs to exist purely so
unit tests could neutralise pool service should not have held. Both
gone.

**Cost accepted:** `TxRunner` one interface with one production implementation,
forever — textbook YAGNI, accepted because it fixes type-width problem
compiler can then police. Does **not** make transactions explicit;
transaction still travels ambiently in `context`.

**Deliberate inconsistency:** `TxRunner` declared once in `platform/database`
rather than per-consumer like every other port. Features already import
`platform/database`, so per-consumer declaration would not have removed
dependency — only duplicated interface five times and generated five
identical mocks.

**One port per backing store:** `TxRunner` narrows what service holds for
_atomicity_; says nothing about how many stores feature talks to, and
nothing stops that number being more than one. `user` first
feature where it is: `user.Repository` its Postgres port, adapted by
`postgres/`, and `user.StatusCache` second, independent port over Redis,
adapted by `redis/`. Rule generalises same way decision 3's
subpackage-per-technology split does — one port per backing store
(`repository.go`, `cache.go`), one adapter subpackage per port —
rather than widening `Repository` to also cover caching, which would have coupled two
stores' failure modes into one interface.

## 6. Modules own their data

Module's SQL may only name tables it owns. Cross-module reads go through
port. `./db/OWNERSHIP.md` lists who owns what and is map
`scripts/check-boundaries.sh` enforces — parsed at run time, so
document and check cannot drift apart.

**Why:** Go-level boundaries worthless if `cart` reaches into `products`
anyway. Before this, four modules crossed in SQL — and `cart` worst,
holding _both_ `ProductLookup` port and `JOIN products` fetching same
five fields, which taught reader port was optional.

**Cost accepted:** two queries where one join would do, and `?in_stock=true`
becomes unimplementable (see limitations).

**Carve-out:** `dashboard` reporting read-model and may read-only join
across anything. Expressing revenue aggregate as cross-module service calls
instead of `GROUP BY` would be slower _and_ less correct.

## 7. Inventory owns stock; product does not

`inventory_levels(product_id, available_stock, reserved_stock)`. `product` reads
availability through batch `InventoryReader` port.

**Why:** product information and stock levels change at different rates and are
edited by different roles. Checkout talks to inventory, never to product. Also
removed genuine concurrency problem: reserving stock used to row-lock `products`,
blocking admin editing product's name for duration of checkout.

**Cost accepted:** creating product then setting its stock is two admin calls.
Alternative — product writing inventory's table inside its own transaction —
is exact violation being removed.

**Shape detail:** `available_stock` _stored_, not derived, so each operation is
single guarded column update and `DeductBatch` touches one column instead of
two. Total on hand derived as `available + reserved`.

## 8. Foreign keys stay; cross-module cascades do not

18 of schema's 25 foreign keys cross module boundaries, and all 18 kept.
Six cross-module `ON DELETE CASCADE` clauses dropped. Counts verified
against `pg_constraint` on migrated database; see `./db/OWNERSHIP.md`.

**Why keep the FKs:** in single database, referential integrity Postgres
enforces beats discipline code review enforces. `products.category_id`
load-bearing in Go — category's delete catches FK violation as backstop.

**Why drop the cascades:** unreachable. `users` and `products`
soft-deleted, so cascade could never fire — while schema implied cart
cleanup that never happened. Lie in schema worse than absence.

## 9. `x/http` owns the wire format

No `json` tag exists on type **this system owns** outside
`internal/modules/*/http/`. Every endpoint owns its request DTO, response DTO
and explicit mapping; those live beside handler that serialises them. Files
inside `http/` split by **handler role**, not one per use case:
`handler.go` for default handler, `admin_handler.go` where routes split
by caller role, and `webhook_handler.go` in `payment`, whose only non-admin
route is gateway callback and which therefore has no `handler.go` at all.
Each has `_test.go` beside it, `package http`, holding both route-level
tests and tests that must reach unexported mappers directly. `routes.go`
holds only `RouteDeps` and `RegisterRoutes`.

`make check-boundaries` enforces tag rule, not file layout. Nothing
checks how handlers distributed across files; what script does
check is `json` tags outside `http` adapter, cross-module table references
in SQL, and one feature importing another's `postgres`/`http`/`redis` package.

Two exemptions, both deliberate and both allowlisted by name in check:

- **`internal/modules/payment/gateway.go`** — `ChargeRequest`/`ChargeResponse`/`RefundRequest`/
  `RefundResponse` are _external_ gateway's wire contract, not ours. Those tags
  describe someone else's API, and `payment/stripe` and `payment/midtrans` marshal
  them on way out. Mapping `Money` down to their plain `int64`+`string` fields
  in those adapters is correct seam, not leak.
- **`internal/platform/paging/`** — shared cursor/offset pagination envelope,
  i.e. transport infrastructure rather than domain model.

Unexplained exemption in lint rule is how rule erodes, so each one
named in `scripts/check-boundaries.sh` with its reason next to it.

**Why:** thirteen `json:"-"` tags were load-bearing security controls —
`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`. Two deleted
characters published password hash. This inverts default: field now
private unless DTO names it. Also makes `x/http` mean something; with tags on
model, core still dictates API and adapter just folder.

**Cost accepted:** ~40 mapper functions, and request DTOs had to split into
core `…Params` type (no tags) plus unexported wire type — otherwise core
would import its own adapter.

## 10. `money.Money`, not `int64` beside a `Currency string`

**Why:** codebase hand-rolled it in two places — currency-consistency
loop in `order.PlaceOrder`, and two-field `Amount != … || Currency != …` compare
in `payment`'s verification path — across **twelve `Currency string` fields** that
could each drift from amount beside them. `Money` makes "amount without its
currency" unrepresentable, and collapses both hand-rolled checks into one
`ErrCurrencyMismatch` from `Add`/`Equal`.

Exactly **one** loose `Currency string` now survives outside adapter, and it
the deliberate exemption in §9: `internal/modules/payment/gateway.go`, external
gateway's own contract. `Money` maps down to its plain `int64`+`string` fields in
`payment/stripe` and `payment/midtrans`, which correct seam.

**Scope: four features — `order`, `payment`, `product`, `cart`.** Those
only ones whose data model carries currency at all. Two deliberately
excluded, and reasons load-bearing rather than bookkeeping:

- **`promotion` stays on `int64`.** `Promotion.Value` _polymorphic_: with
  `TypePercentage` it percentage (`service.go:167` guards `value > 100`),
  with `TypeFixedAmount` it minor units. `money.New(10, "USD")` to mean "10%"
  would be value object asserting something false. And promotion has no
  currency field anywhere, so even its genuinely-monetary `MinOrderAmount`,
  `MaxDiscount` and `CouponUsage.Discount` have nothing to pair with — inventing
  one would fabricate data system never captured.
- **`dashboard` stays on `int64`.** Aggregates revenue across orders and has no
  currency field, so any single currency would be guess.

Neither exclusion observable: both features emit zero `currency` keys on
wire today, exactly because domain has none either.

**Cost accepted:** explicit two-column mapping in every `postgres` adapter, and
flattening in every response DTO. `Money` carries no `json` tag and implements no
`sql.Scanner` on purpose: serialisation each adapter's job. Not
fastidiousness — `cart`'s response has `total` with **no** sibling currency
while its nested items carry `price` _and_ `currency`, and `order` inconsistent
in opposite direction (currency at order level, none on line items). A
self-marshalling `Money` would simultaneously add key to first group and
double-emit it for second. One type cannot satisfy both; only adapter can
decide. Also no float constructor.

**No `Div`:** dividing money needs stated rounding and remainder-allocation
policy — who gets leftover cent when splitting 10 three ways. Silently picking
one is how rounding bugs enter ledger. When split needed, add named method
that states its policy in its name.

**Two seams where `Money` deliberately stops.** Both places reader will
otherwise read as oversight:

1. **`order.CouponReserver`** (`internal/modules/order/ports.go`) still passes `orderSubtotal int64`
   and returns `discountAmount int64`. Its implementer is `promotion`, which has no
   currency to honour `Money` with. Pairing happens on order's side of
   seam — `internal/modules/order/service.go` passes `subtotal.Amount` and rebuilds
   `money.New(discount, subtotal.Currency)` — which also where clamp policy
   lives: `max(subtotal-discount, 0)`, so over-large coupon cannot produce
   negative charge. `Money.Sub` deliberately does not decide that, so clamp
   plain arithmetic on amounts with comment saying why.
2. **`cart.Cart.Total()` returns `(money.Money, error)`.** Total used to be
   summed inside HTTP adapter, which both wrong owner for domain
   calculation and impossible once sum can fail.

**One observable behaviour change came out of this.** Mixed-currency cart now
returns **400** from `GET /cart`; previously returned 200 with amounts added
together, denominated in nothing. Nothing prevents such cart — prices
per-product and `AddItem` does not constrain them — and checkout already rejected
it, so this makes `GET /cart` agree with `PlaceOrder`. Error wraps
`apperror.ErrBadRequest` alongside `money.ErrCurrencyMismatch`, because
`ErrCurrencyMismatch` alone matches no case in `response.HandleErr` and would
surface as 500 for what plainly user input. `Total()` folds **sellable lines
only**, so archived line in another currency still yields clean 200 — and
empty cart yields `total: 0`, not error.

## 11. Integration tests stay next to their code; only e2e is centralised

**Why:** `go test ./...` runs 19 test binaries concurrently against one shared
container. Collapsing them into single `test/integration` package makes them
sequential, and `t.Parallel()` cannot recover it because subtests share
database. `test/e2e` exists for checkout and refund sagas — flows spanning
five modules no single feature package can own.

**Cost accepted:** 19 `TestMain` functions instead of one.

## 12. Log attributes travel in the context, not in signatures

A service that logs `request_id` has no business knowing what an HTTP
request is. The alternative — threading the value down as a parameter, or
handing every layer a pre-built logger — makes a transport concern part of
fourteen service APIs.

So `logger.WithAttrs(ctx, ...)` stores attributes in the context and
`logger.ContextHandler` merges them into every record. `logger.Setup`
installs the wrapper, so every logger in both binaries has it. Services
keep their constructor-injected `*slog.Logger` and their existing
`InfoContext(ctx, ...)` calls unchanged. Only the four edges that name an
attribute gained one `logger.WithAttrs` line each — two of them,
`payment.Service.Process` and `notification.Service.Process`, inside a
feature package. Every other call, in every other feature, needed no
change to start carrying the context's attributes.

Two details are load-bearing rather than stylistic. `ContextHandler`
overrides `WithAttrs` and `WithGroup`, because the methods promoted from
the embedded handler return the *inner* handler — `logger.With(...)` would
otherwise produce a logger that silently emits no context attributes.
And `WithAttrs` clips the slice before appending, because two contexts
derived from one parent would otherwise share a backing array and
overwrite each other.

**The cost:** you can no longer read a single log call and know everything
it emits. `order.Service.ExpireStale`'s
`s.logger.ErrorContext(ctx, "failed to expire order", slog.String("order_id", o.ID.String()), slog.String("error", err.Error()))`
also emits `runner`, because it runs inside the payment runner's per-tick
sweep, and nothing at that line names it. In exchange, 32 repeated
attributes are gone and `request_id` reaches code that has never heard of
HTTP.

## 13. A `<feature>/contract/` package publishes the structs that cross a boundary

Seven of fourteen modules — `auth cart inventory order payment product user` —
have a `contract/` package: `user/contract.User`, `product/contract.Product`,
`inventory/contract.StockState`, `order/contract.Order`, `payment/contract.ChargeRequest`,
`cart/contract.Cart`, `auth/contract.Claims`, and their siblings. Each package
imports no module and no platform package, so importing one can never pull the
producer's implementation along with it — a consumer takes the type by value
and never learns how it is built. A port still names the type it needs
(`auth.UserProvider.GetByID(ctx, id) (usercontract.User, error)`); the
contract package supplies only the shape, never the interface — that stays
declared by the consumer, per decision 2.

**Why:** decision 2's trick — a producer's own method already named what the
consumer's port asked for, so `promotion.Service` satisfies
`payment.CouponReleaser` with no adapter at all — works for scalars and for
interfaces a producer already implements. It does not work when what crosses
is a struct: two modules cannot each declare their own `User` and have the
compiler agree the two are the same type. Every module that names a struct
type in a port it does not own needed exactly one published type for that
struct; the other seven modules never pass one across a port and have no
`contract/` to show for it.

**Cost accepted:** a module now has a published surface. Before, changing an
internal struct's shape was a one-module diff; changing `user/contract.User`
is now a change every consumer of `user` must absorb, whether or not the field
they care about moved. The alternative this replaced — structurally-identical
types declared in both modules plus a mapping function between them — paid the
same cost at every call site instead of at the one file that changed;
`contract/` moves it from many places to one, but does not remove it. See
`ARCHITECTURE-LIMITATIONS.md` for where that published surface can go wrong on
its own account.

## 14. A module is a business boundary containing vertical slices

`internal/modules/shipping/` is the reference — first of fourteen modules
moved to this shape, thirteen still to go. It holds `domain/` (its aggregate
and rules — `Shipment`, `ShipmentStatus`, `CanShipOrder` — module-private by
convention, not by any check: nothing outside `shipping` needs it, so nothing
imports it), `module.go` (constructs the slices, imports no transport
package), `http/routes.go` (holds only `RouteDeps` and `RegisterRoutes`,
assembling each slice's own `http` package), a `contract/` **only if another
module consumes a struct from it** (`shipping` has none: only
`internal/bootstrap` and `internal/transport` import it at all, and both are
the wiring layer, not a consumer), and one package per use case — `query`,
`create`, `updatetracking`, `deliver`.

A slice owns its use case (a `Command` with one `Execute`, for the three that
write; a `Reader` named for what it answers, for the one that only reads —
`query.Reader.GetByOrderIDForUser`), its own storage port, its `postgres/`
adapter and its `http/` adapter. "Own" is literal: `updatetracking.Repository`
and `deliver.Repository` both need `GetByID`; each declares it rather than
sharing one. A slice declares a port for anything it does not implement
itself — another module's capability or a sibling slice's — and `module.go`
wires it. Three of shipping's four slices declare one (`query.OrderPort`,
`create.OrderPort`, `deliver.OrderPort`, each satisfied by `order.Service` by
name-match, same trick as decision 2); `updatetracking` declares none and
takes no `TxRunner` either, because it changes two fields on a row it already
fetched and writes it back — there is nothing outside itself to ask.

**Why:** a use case's whole implementation is one directory, and what it
depends on is its own `ports.go`, or its absence. `ls
internal/modules/shipping/` is the module's use-case list, and a slice with no
`ports.go` reaches nothing beyond itself — `updatetracking` is that case, not
an oversight.

**Cost accepted:** three packages per slice (root, `postgres/`, `http/`), so
shipping alone went from 3 packages, layered, to 15, sliced. `module.go`
imports each slice's own package plus that slice's `postgres/` adapter;
`http/routes.go` imports each slice's `http/` adapter and reaches its command
or reader through `shipping.Module`'s fields instead of importing the slice
package directly. Every slice's adapter is named `postgres` or `http`, same as
every other slice's, so importing it needs an alias, not merely permits one —
four aliased imports in each file today, one per slice. Response DTOs are
duplicated across slices that return the same shape: `query`, `create`,
`updatetracking` and `deliver` each declare their own unexported
`shipmentResponse` rather than share one — deliberately, so one endpoint's new
field cannot appear in another's output. The other thirteen modules pay the
same multiplier once sliced, scaled to however many use cases each turns out
to need.

---

# Rejected

Each considered seriously. Reasoning recorded because "why
not" is half that usually gets lost.

## `internal/shared/`

**Rejected.** With `address`, `events`, and `ids` all cut, would have held one
package — not enough to earn namespace level.

Stronger reason: **`shared` is one directory name that attracts entropy.**
Folder with no owner, so everything eventually lands there. Template
shipping `shared/` with "keep this small!" comment hands reader loaded
gun with warning label. Absent directory cannot be filled. `money` lives at
`internal/money`, and `apperror` stayed where it was — moving it would have
touched nearly every file for nothing.

If third genuinely cross-module vocabulary type ever appears, introduce it
then; move mechanical.

## Typed IDs (`ProductID`, `UserID`)

**Rejected**, despite real hazard: 25 functions take two or three adjacent
`uuid.UUID` arguments, including `HasDeliveredOrder(ctx, userID, orderID, productID)`
and `Delete(ctx, requesterID, targetID)` where transposition compiles and
silently acts on wrong record.

**Why not:** Go makes this far more expensive than Java or C#. `type ProductID uuid.UUID`
inherits **nothing** — `String`, `MarshalJSON`, `UnmarshalJSON`, `Scan`, and
`Value` all need reimplementing per type, or struct-embedding trick which
changes every construction site and needs pgx-codec spike. Across 173 sites.
And then must choose between `shared/ids` — shared kernel every module
imports, directly against decision 6 — or per-module IDs with conversion
boilerplate in every adapter.

**Chosen instead:** params structs on risky functions. Named fields make
transposition impossible at call site, cost no marshaling work, and create no
shared kernel.

## `shared/address`

**Rejected.** Exactly one module defines address type (`internal/modules/order/address.go`), and
`shipping` has none — shipments key off `order_id`. Promoting single-consumer
type to shared package is opposite of what "shared" should mean.

## `shared/events` / a domain event bus

**Rejected.** No event bus exists, so this was new subsystem rather than
relocation. One case that would have justified it — cascading cleanup on
`user.deleted` — evaporated on discovering users soft-deleted, so nothing
needs to react to deletion that never hard-deletes.

## Multi-warehouse inventory

**Rejected as out of scope.** `inventory_levels` keyed by `product_id` alone.
`warehouse` column is not column — breaks single-statement batch
reserve four different ways and needs allocation policy, per-line reservation
records, `warehouse_id` on `order_items`, and split shipments in `shipping`. Four
modules and new table.

Nullable `warehouse` column every query ignores would be **worse than
absent**, because reader assumes it works. See limitations for full
breakdown.

## `x/grpc/`

**Rejected.** No gRPC dependency, no service definition, no caller.
Empty `grpc/` directory in template is dead scaffolding readers copy and
never fill. Add when there is proto.

## `x/webhook/` as a package

**Rejected** in favour of `internal/modules/payment/http/webhook_handler.go`. Webhook _is_ HTTP;
splitting it out fragments route registration across two packages for one feature.
Things that actually need protecting — no JWT middleware, raw body access,
signature verification — already handled by route group.

## `notification/worker/`

**Rejected.** `notification.Service` satisfies `jobs.Processor` directly. Writing
`worker.NewProcessor(svc)` that returns `svc` to fill slot would be ceremony
teaching opposite of decision 4.

## `platform/uuid`

**Rejected.** Wrapper around `google/uuid` that adds nothing. Wrapping stable
third-party library "in case we swap it" is abstraction habit this codebase
trying not to teach.

## `platform/clock`

**Rejected for now.** Real case existed — `payment` computes retry backoff
with jitter and job runner leases against wall-clock deadlines, so `Clock`
port would make both testable without sleeping. Cut because no test currently
needs it, and port with no consumer speculative. Worth revisiting first
time backoff test resorts to `time.Sleep`.

Note Go 1.25's `synctest` not alternative here: panics on pool
`Acquire`, so time-faking has to come from injected seam.

## `test/integration/`

**Rejected.** See decision 11 — would make suite slower, not faster, and
break colocation Go's tooling built around.

## `product_view` read model

**Rejected for now**, and it documented escape hatch for decision 6's main
cost. Read model with no consumer speculative infrastructure; moment
storefront needs `?in_stock=true`, this the answer. See limitations.

## Backward compatibility

**Explicitly rejected.** Template, not deployed service, so API shapes
changed wherever better design demanded — notably `reserved_quantity`
leaving public product response (published live order velocity per SKU to
any unauthenticated caller) and `stock_quantity` leaving product's write DTOs.

## A logger in the context

**Rejected.** `ctx` carrying a `*slog.Logger` that middleware built with
`.With(...)`, fetched at each call site as `logger.FromContext(ctx).Info(...)`.
It uses `With` literally, which is what makes it tempting.

It deletes the constructor-injected logger and so hides a real dependency:
a service's signature would stop saying that it logs. Every one of ~143
call sites changes, and every one needs a fallback for the contexts that
have no logger — worker jobs, tests, anything below a
`context.Background()`. `sloglint`'s `no-global: all` forbids the obvious
fallback. `ContextHandler` gets the same output with no call-site change
and no nil case.

## OpenTelemetry wiring

**Rejected as scope.** `trace_id` and `span_id` are the canonical things to
carry contextually, and `go.opentelemetry.io/otel` is already in `go.mod`.
It is **indirect**: pulled in by a dependency, imported nowhere.

A tracer provider, an OTLP exporter, sampler configuration, `otelhttp`
middleware, new config invariants and a shutdown hook in both binaries is a
tracing feature, not a logging refactor. The seam costs
nothing to leave open: whoever adds tracing calls
`logger.WithAttrs(ctx, slog.String("trace_id", sc.TraceID().String()))` in
their own middleware and every existing log call picks it up.
