# Architecture decisions

Why this codebase is shaped the way it is — including the things it deliberately
does **not** do, because a structure is only teachable if the roads not taken are
visible.

Read `./ARCHITECTURE-LIMITATIONS.md` (forthcoming) for the bills these decisions
come with, and `./db/OWNERSHIP.md` (forthcoming) for the table ownership map.

---

## 0. This repository is a template, so the structure is the product

Every decision below is judged by **what it teaches**, not by what is cheapest to
maintain. That is unusual and it changes the calculus: where a product codebase
would keep a working shortcut, this one pays for a boundary the compiler can
enforce, because a reader will copy whatever they find here into a real system.

Two consequences worth naming, since they look like mistakes otherwise:

- The `postgres`/`http` subpackage split costs ~26 aliased imports in the
  composition file. In a product that would be hard to justify. Here it is the
  point: a physical boundary teaches the port/adapter distinction in a way a file
  naming convention cannot.
- Where a rule exists, it is machine-checked (`make check-boundaries`). A rule
  that lives only in a README rots, and a template that ships a rotted rule
  teaches the rot.

Backward compatibility is explicitly **not** a goal. API shapes were changed
freely where the better design demanded it.

---

## 1. Feature modules, not layers

`internal/order/` holds order's domain types, service, repository interface, and
the ports it needs — not `internal/domain/order` plus `internal/application/order`
plus `internal/infrastructure/order`.

**Why:** `payment.NewService()` and `payment.Repository` read naturally in Go;
`application.NewService()` and `domain.Repository` put a layer name in every
import and tell you nothing about what the code is for. Layered trees also scatter
one change across three directories.

**Cost accepted:** a feature package is larger than any single layer file would be.

## 2. Ports live with the consumer

`order/inventory.go` declares `InventoryReserver` — the interface *order* needs.
`inventory` does not publish it. `bootstrap` supplies an adapter.

**Why:** no module imports another, so the dependency graph has no cycles by
construction and each module's port list is exactly the API it would need if
extracted. It also pays off immediately: because the interfaces are declared
narrow at the consumer, `promotion.Service` satisfies `payment.CouponReleaser`
directly and `notification.Service` satisfies `jobs.Processor` directly — two
adapters that did not need to be written.

**Cost accepted:** structurally-identical types declared in two places, plus a
mapping adapter where the shapes differ.

## 3. Adapters are subpackages named for their technology

`payment/postgres`, `payment/http`, `payment/stripe`, `payment/midtrans`,
`payment/worker`.

**Why:** the dependency rule becomes a compile error rather than a convention —
`payment` cannot import `payment/postgres` without a cycle, so SQL physically
cannot leak into the core.

**Cost accepted:** 13 packages named `postgres` and 13 named `http`, so every
composition site needs import aliases (`paymentpg`, `paymenthttp`). The cost
concentrates in one file, deliberately.

## 4. Adapter subpackages exist only where adaptation is needed

`payment/` has six subpackages; `wishlist/` has two. `notification` has **no**
`worker/` package because its `Service` satisfies `jobs.Processor` directly.

**Why:** a pass-through package created to make trees look uniform teaches that
adapters are bureaucracy. The absence is the lesson.

**Cost accepted:** you cannot predict a module's shape without looking.

## 5. Services take `database.TxRunner`, never `*pgxpool.Pool`

**Why:** the pool was only ever passed to `database.WithTx` — zero direct
queries. So it was an *atomicity* dependency wearing a database type, 100× wider
than the need, and nothing stopped a service from adding `s.pool.Query(...)`. It
also forced a `WithTestTx` helper and ~20 `noopDBTX{}` stubs to exist purely so
unit tests could neutralise a pool the service should not have held. Both are
gone.

**Cost accepted:** `TxRunner` is one interface with one production implementation,
forever — textbook YAGNI, accepted because it fixes a type-width problem the
compiler can then police. It does **not** make transactions explicit; the
transaction still travels ambiently in the `context`.

**Deliberate inconsistency:** `TxRunner` is declared once in `platform/database`
rather than per-consumer like every other port. Features already import
`platform/database`, so a per-consumer declaration would not have removed the
dependency — only duplicated the interface five times and generated five
identical mocks.

## 6. Modules own their data

A module's SQL may only name tables it owns. Cross-module reads go through a
port. See `./db/OWNERSHIP.md` (forthcoming).

**Why:** Go-level boundaries are worthless if `cart` reaches into `products`
anyway. Before this, four modules crossed in SQL — and `cart` was the worst,
holding *both* a `ProductLookup` port and a `JOIN products` fetching the same
five fields, which taught a reader the port was optional.

**Cost accepted:** two queries where one join would do, and `?in_stock=true`
becomes unimplementable (see limitations).

**Carve-out:** `dashboard` is a reporting read-model and may read-only join
across anything. Expressing a revenue aggregate as cross-module service calls
instead of a `GROUP BY` would be slower *and* less correct.

## 7. Inventory owns stock; product does not

`inventory_levels(product_id, available_stock, reserved_stock)`. `product` reads
availability through a batch `InventoryReader` port.

**Why:** product information and stock levels change at different rates and are
edited by different roles. Checkout talks to inventory, never to product. It also
removed a genuine concurrency problem: reserving stock used to row-lock `products`,
blocking an admin editing a product's name for the duration of a checkout.

**Cost accepted:** creating a product then setting its stock is two admin calls.
The alternative — product writing inventory's table inside its own transaction —
is the exact violation being removed.

**Shape detail:** `available_stock` is *stored*, not derived, so each operation is
a single guarded column update and `DeductBatch` touches one column instead of
two. Total on hand is derived as `available + reserved`.

## 8. Foreign keys stay; cross-module cascades do not

15 of 24 foreign keys cross module boundaries and are kept. Six cross-module
`ON DELETE CASCADE` clauses were dropped.

**Why keep the FKs:** in a single database, referential integrity Postgres
enforces beats discipline a code review enforces. `products.category_id` is
load-bearing in Go — category's delete catches the FK violation as a backstop.

**Why drop the cascades:** they were unreachable. `users` and `products` are
soft-deleted, so the cascade could never fire — while the schema implied a cart
cleanup that never happened. A lie in the schema is worse than an absence.

## 9. `x/http` owns the wire format

No `json` tag exists outside `internal/*/http/`. Every endpoint owns its request
DTO, response DTO, and explicit mapping, one use case per file.

**Why:** thirteen `json:"-"` tags were load-bearing security controls —
`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`. Two deleted
characters published a password hash. This inverts the default: a field is now
private unless a DTO names it. It also makes `x/http` mean something; with tags on
the model, the core still dictates the API and the adapter is just a folder.

**Cost accepted:** ~40 mapper functions, and request DTOs had to split into a
core `…Params` type (no tags) plus an unexported wire type — otherwise the core
would import its own adapter.

## 10. `money.Money`, not `int64` beside a `Currency string`

**Why:** the codebase was hand-rolling it in two places — a currency-consistency
loop in `order.PlaceOrder` and a two-field equality check in `payment` — across
~50 amount fields that could drift from their currency. `Money` makes "amount
without its currency" unrepresentable.

**Cost accepted:** explicit two-column mapping in every `postgres` adapter, and
flattening in every response DTO. `Money` carries no `json` tag and implements no
`sql.Scanner` on purpose: serialisation is each adapter's job.

**No `Div`:** dividing money needs a stated rounding and remainder-allocation
policy. Silently picking one is how rounding bugs enter a ledger.

## 11. Integration tests stay next to their code; only e2e is centralised

**Why:** `go test ./...` runs 17 test binaries concurrently against one shared
container. Collapsing them into a single `test/integration` package makes them
sequential, and `t.Parallel()` cannot recover it because subtests share a
database. `test/e2e` exists for the checkout and refund sagas — flows spanning
five modules that no single feature package can own.

**Cost accepted:** 17 `TestMain` functions instead of one.

---

# Rejected

Each of these was considered seriously. The reasoning is recorded because "why
not" is the half that usually gets lost.

## `internal/shared/`

**Rejected.** With `address`, `events`, and `ids` all cut, it would have held one
package — not enough to earn a namespace level.

The stronger reason: **`shared` is the one directory name that attracts entropy.**
It is the folder with no owner, so everything eventually lands there. A template
shipping a `shared/` with a "keep this small!" comment hands the reader a loaded
gun with a warning label. An absent directory cannot be filled. `money` lives at
`internal/money`, and `apperror` stayed where it was — moving it would have
touched nearly every file for nothing.

If a third genuinely cross-module vocabulary type ever appears, introduce it
then; the move is mechanical.

## Typed IDs (`ProductID`, `UserID`)

**Rejected**, despite a real hazard: 25 functions take two or three adjacent
`uuid.UUID` arguments, including `HasDeliveredOrder(ctx, userID, orderID, productID)`
and `Delete(ctx, requesterID, targetID)` where a transposition compiles and
silently acts on the wrong record.

**Why not:** Go makes this far more expensive than Java or C#. `type ProductID uuid.UUID`
inherits **nothing** — `String`, `MarshalJSON`, `UnmarshalJSON`, `Scan`, and
`Value` all need reimplementing per type, or the struct-embedding trick which
changes every construction site and needs a pgx-codec spike. Across 173 sites.
And then you must choose between `shared/ids` — a shared kernel every module
imports, directly against decision 6 — or per-module IDs with conversion
boilerplate in every adapter.

**Chosen instead:** params structs on the risky functions. Named fields make a
transposition impossible at the call site, cost no marshaling work, and create no
shared kernel.

## `shared/address`

**Rejected.** Exactly one module defines an address type (`order/address.go`), and
`shipping` has none — shipments key off `order_id`. Promoting a single-consumer
type to a shared package is the opposite of what "shared" should mean.

## `shared/events` / a domain event bus

**Rejected.** No event bus exists, so this was a new subsystem rather than a
relocation. The one case that would have justified it — cascading cleanup on
`user.deleted` — evaporated on discovering users are soft-deleted, so nothing
needs to react to a deletion that never hard-deletes.

## Multi-warehouse inventory

**Rejected as out of scope.** `inventory_levels` is keyed by `product_id` alone.
A `warehouse` column is not a column — it breaks the single-statement batch
reserve four different ways and needs an allocation policy, per-line reservation
records, `warehouse_id` on `order_items`, and split shipments in `shipping`. Four
modules and a new table.

A nullable `warehouse` column that every query ignores would be **worse than
absent**, because a reader assumes it works. See limitations for the full
breakdown.

## `x/grpc/`

**Rejected.** There is no gRPC dependency, no service definition, and no caller.
An empty `grpc/` directory in a template is dead scaffolding that readers copy and
never fill. Add it when there is a proto.

## `x/webhook/` as a package

**Rejected** in favour of `payment/http/webhook.go`. A webhook *is* HTTP;
splitting it out fragments route registration across two packages for one feature.
The things that actually need protecting — no JWT middleware, raw body access,
signature verification — are already handled by the route group.

## `notification/worker/`

**Rejected.** `notification.Service` satisfies `jobs.Processor` directly. Writing
`worker.NewProcessor(svc)` that returns `svc` to fill a slot would be ceremony
teaching the opposite of decision 4.

## `platform/uuid`

**Rejected.** A wrapper around `google/uuid` that adds nothing. Wrapping a stable
third-party library "in case we swap it" is the abstraction habit this codebase is
trying not to teach.

## `platform/clock`

**Rejected for now.** There was a real case — `payment` computes retry backoff
with jitter and the job runner leases against wall-clock deadlines, so a `Clock`
port would make both testable without sleeping. Cut because no test currently
needs it, and a port with no consumer is speculative. Worth revisiting the first
time a backoff test resorts to `time.Sleep`.

Note that Go 1.25's `synctest` is not an alternative here: it panics on pool
`Acquire`, so time-faking has to come from an injected seam.

## `test/integration/`

**Rejected.** See decision 11 — it would make the suite slower, not faster, and
break the colocation Go's tooling is built around.

## `product_view` read model

**Rejected for now**, and it is the documented escape hatch for decision 6's main
cost. A read model with no consumer is speculative infrastructure; the moment a
storefront needs `?in_stock=true`, this is the answer. See limitations.

## Backward compatibility

**Explicitly rejected.** This is a template, not a deployed service, so API shapes
changed wherever the better design demanded it — notably `reserved_quantity`
leaving the public product response (it published live order velocity per SKU to
any unauthenticated caller) and `stock_quantity` leaving product's write DTOs.
