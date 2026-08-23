# Architecture decisions

Why this codebase shaped this way — including things it deliberately
does **not** do, because structure only teachable if roads not taken
visible.

Read `./ARCHITECTURE-LIMITATIONS.md` for bills these decisions
carry, and `./db/OWNERSHIP.md` for table ownership map — which
`make check-boundaries` parses, so enforced not merely asserted.

**A refactor is in progress, collapsing slices into per-module services.**
See `REFACTOR-PLAN.md`. The decisions below were written while all fourteen
modules were sliced; that stopped being true at Task 6, which flattened
`wishlist` first, and each further flatten task removes one more. Read every
"all fourteen", "every module is sliced" and per-slice count below as the
pre-refactor state the decision was made against, not the present tree —
individual symbol names and file paths are kept current as they change, but
these module-count claims are stale until the refactor completes.

---

## 0. This repository is a template, so the structure is the product

Every decision below judged by **what it teaches**, not by what cheapest to
maintain. Unusual, and changes calculus: where product codebase
would keep working shortcut, this one pays for boundary compiler can
enforce, because reader will copy whatever they find here into real system.

Two consequences worth naming, since they look like mistakes otherwise:

- `postgres`/`http` subpackage split costs an import alias per adapter,
  wherever that adapter is wired: 6 in the composition root
  (`internal/bootstrap/app.go`), 1 to 11 per module in each `module.go`, and
  3 to 5 per feature across the 14 files of
  `internal/transport/http/routes/`. In product, hard to justify. Here it the
  point: physical boundary teaches port/adapter distinction in way file
  naming convention cannot.
- Where rule exists, machine-checked (`make check-boundaries`). Rule
  living only in README rots, and template shipping rotted rule
  teaches rot.

Backward compatibility explicitly **not** goal. API shapes changed
freely where better design demanded.

---

## 1. Feature modules, not layers

`internal/modules/order/` holds everything order owns — not
`internal/domain/order` plus `internal/application/order` plus
`internal/infrastructure/order`. This decision is about that outer
boundary only; decision 14 covers what now sits inside it — a `domain/`,
one `module.go`, and a `usecase/` directory holding one package per use
case, in place of the single `service.go`/`repository.go` this decision
originally described. Both are true at once: `order` is still one feature
package, not three layer packages: it is now also a boundary containing
several slice packages, not one flat package.

**Why:** `charge.New()` and `payment/usecase/charge.Repository` read
naturally in Go; `application.NewService()` and `domain.Repository` put the
layer name in every import and tell nothing about what the code is for.
Layered trees also scatter one change across three directories.

**Cost accepted:** a feature is a directory tree, not a file — bigger than
any single layer file was, and now two levels deeper than the flat feature
package this decision first proposed.

**Why the `modules/` wrapper:** 14 feature packages sit under
`internal/modules/`, one directory below old home, so
`scripts/check-boundaries.sh` reads feature list straight off
filesystem instead of maintaining denylist of everything under `internal/`
that _not_ feature. Denylist it replaced already drifted once —
`money` missing from it, so shared value object briefly treated
as module subject to ownership checks. Directory right by
construction cannot drift way list of exceptions can.

## 2. Ports live with the consumer

The consumer is now a slice, or the module composing several of them.
`internal/modules/payment/usecase/webhook/ports.go` declares `OrderUpdater`
— the interface `webhook` alone needs from order. `order` does not
publish it; `webhook` names exactly what it needs and something else
satisfies it. `order/module.go` is the
other shape: `place`, `cancel` and `expire` all need inventory, so
`order`'s port lives in `module.go` instead, as one interface plus one
`Deps` field every slice that needs it shares — grouping at the module
level when several slices would otherwise each declare the same
interface. Naming a port file after the dependency instead of `ports.go`
— an alternative this decision once allowed — no longer happens anywhere
in the tree: every port file, at either level, is `ports.go`.

**Why:** no module imports another's implementation, so the dependency
graph has no cycles by construction and each slice's — or module's — port
list is exactly the API it would need if extracted. Pays off immediately:
because interfaces are declared narrow at the consumer,
`promotion.Service` satisfies both `order.CouponPort` and
`payment.CouponPort` directly, and notification's `jobs.Worker` satisfies
`platform/jobs.Processor` directly — adapters never needed writing.

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

Now two levels deeper, per slice:
`internal/modules/payment/usecase/charge/postgres`,
`internal/modules/payment/usecase/webhook/postgres`,
`internal/modules/payment/usecase/webhook/http`.
`payment/gateway/stripe`, `payment/gateway/midtrans` and `payment/gateway/mock`
are the exception that proves the rule at a different scope — an adapter
family for one outbound port shared by two slices, still named for its
technology, just not nested inside either slice that uses it.

**Why:** dependency rule becomes a compile error, not a convention — a
slice cannot import its own `postgres/` without a cycle, so SQL physically
cannot leak into its command or reader.

**Cost accepted:** 64 packages named `postgres` and 53 named `http` under
`internal/modules` today, one named `redis` — re-run `find internal/modules
-type d -name postgres | wc -l` (and `http`, `redis`) rather than trust
these; they move with every slice added. The `http` count fell from 67 when
decision 15 moved the 14 feature route tables into
`internal/transport/http/routes/`: what is left under a module is exactly one
`http/` per slice that has a route, and no route table anywhere. Every
module's own `module.go` needs one import alias per slice adapter it wires
(`addpg`, `querypg`, …) — from 1 (`auth`) to 11 (`order`) aliases per file
today, counted with
`grep -cE '^\s+[a-z]+ "github.com/residwi' internal/modules/<feature>/module.go`.
The slice `http/` adapters are aliased somewhere else entirely now:
`internal/transport/http/routes/<feature>.go` imports each of its feature's
slice handlers (`addhttp`, `queryhttp`, …), 3 to 5 per file, and
`router.go` itself is down to a single aliased import — the dev-only mock
gateway's registrar — because it imports `routes` unaliased and calls one
function per feature. The composition root, `internal/bootstrap/app.go`,
escapes most of this: for a still-sliced module it imports the module by
its unaliased root package and lets `module.go` wire that module's own
adapters. Six aliased imports (`ordercancel`, `ordercancelpg`,
`ordertransition`, `ordertransitionpg`, `orderquery`, `orderquerypg`) used
to survive there regardless, kept only because the order/payment cycle
forced bootstrap to build three pieces of `order` two levels past its own
module boundary before `order.New` could run. With `order` needing nothing
from `payment` any more, `order.New` runs first and hands `payment.New` its
own `Module.Transition`, `Module.Cancel` and `Module.Query` by name-match,
so none of the six is needed — down from the double digits a flat, unsliced
`app.go` used to carry. A flattened module reopens one alias here *per
adapter package it owns*, not one per module: `wishlist` (Task 6) owns one
store, so it costs exactly one, `wishlistpg`; `checkout`, a flattened module
that owns no store of its own, costs zero — `app.go` imports it unaliased;
`user`, when it flattens, will cost two (`userpg` plus a cache alias), since
`user/usecase/query` is the one slice in the repo backed by both Postgres and
Redis today. A module with no `module.go` left has nowhere else to wire its
own adapter — see `ARCHITECTURE-LIMITATIONS.md`'s "composition site" entry,
and read this count the same way as the others, stale until every module has
flattened. Cost still concentrates in one file per module and one per
feature's routes, deliberately — just no longer in one file for the whole
binary.

## 4. Adapter subpackages exist only where adaptation is needed

True at the slice level now, and the evidence is sharper for it: `auth`
has **no** `postgres/` anywhere in the module — none of its four slices
stores anything, because the one thing `auth` needs from storage
(`UserProvider.GetByID`) is `user`'s to hold. `user/usecase/query` is the
one slice in the whole repo with two backing stores, `postgres/` and
`redis/` — every other slice in every other module has at most one.
`dashboard` has no `ports.go` at all, in any of its three slices: nothing it
does reaches outside the module, so there is no cross-module dependency for
a port to name. `notification` has **no** `worker/` package because its
`jobs/` package's `Worker` satisfies `platform/jobs.Processor` directly —
one value does both roles `payment` needs three packages for (`jobs.Queue`
the queue, `jobs.Dispatcher` the processor, and `worker/` the wrapper that
also drives order's housekeeping sweep).
`contract/` is not counted as an adapter — it adapts no technology,
decision 13 covers it on its own terms, and a module gets one
independently of how many adapters it needs.

**Why:** a pass-through package created to make the tree look uniform
teaches that adapters are bureaucracy. Absence is the lesson, and it reads
the same way whether the module doing without is one flat feature or,
now, one slice inside a sliced one.

**Cost accepted:** cannot predict a module's shape, or a slice's, without
looking.

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
nothing stops that number being more than one. `user` is the one module
where it is: `user.Repository` its Postgres port, adapted by
`adapter/postgres/`, and `user.StatusCache` second, independent port over
Redis, adapted by `adapter/redis/`. Rule generalises same way decision 3's
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
single guarded column update and `Deduct` touches one column instead of
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

## 9. `<slice>/http` owns the wire format

No `json` tag exists on a type **this system owns** outside
`internal/modules/<feature>/usecase/<slice>/http/` — two directories deeper
than this decision first drew it, now that every feature is slices under a
`usecase/` and every slice's handler is the only thing left in a module that
knows about HTTP. Every endpoint owns its request DTO, response DTO and
explicit mapping; those live beside the handler that serialises them.
Files inside a slice's `http/` split by **handler role**, not one per use
case: `handler.go` for the default handler, `admin_handler.go` where that
slice's routes split by caller role, and `webhook_handler.go` in
`payment/usecase/webhook`, whose only route is the gateway callback and
which therefore has no `handler.go` at all. Each has a `_test.go` beside it,
`package http`, holding both route-level tests and tests that must reach
unexported mappers directly. **No feature has a root `http/` any more**: the
`routes.go` that used to sit there is gone, and the URL it declared lives in
`internal/transport/http/routes/<feature>.go` instead — decision 15.

**Response DTOs are duplicated across slices on purpose.**
`order/usecase/query` and every other slice that returns an order all
declare their own unexported response type rather than share one --
`checkout/adapter/http` does the same for the two order-returning
endpoints that moved out of `order/usecase/place` and the now-deleted
`order/usecase/retrypayment` (decision 15's `place`/`retrypayment` move),
even where two shapes happen to match today: `checkout/adapter/http`'s own
`orderResponse` and `order/usecase/query/http`'s are structurally
identical right now. This is the
same trade decision 14 makes explicit for shipping's four slices — one
endpoint's new field cannot leak into another's response by sharing its
struct — generalised to all fourteen modules now that all fourteen are
sliced. Someone will read the duplication as an oversight and try to
collapse it into one shared type; that is the one thing not to do here.

`make check-boundaries` enforces the tag rule, not the file layout.
Nothing checks how handlers are distributed across files; what the
script checks is `json` tags outside a slice's `http/` (check 1),
cross-module table references in SQL (check 3), and a module importing
anything from another module beyond its `contract/` (check 4) — no
longer phrased as naming a sibling's `postgres`/`http`/`redis` package
specifically, since check 4 also catches a bare root-package import that
never touched an adapter.

Two exemptions, both deliberate and both allowlisted by name in the check:

- **`internal/modules/payment/gateway/gateway.go`** — `ChargeRequest`/`ChargeResponse`/`RefundRequest`/
  `RefundResponse` are _external_ gateway's wire contract, not ours. Those tags
  describe someone else's API, and `payment/gateway/stripe` and `payment/gateway/midtrans`
  marshal them on the way out. Mapping `Money` down to their plain `int64`+`string`
  fields in those adapters is a correct seam, not a leak.
- **`internal/transport/http/response/response.go`** — the shared envelope
  every handler, in every slice, writes through: transport infrastructure,
  not a domain model, the same role `internal/platform/paging/`'s
  cursor/offset envelope plays one layer down.

Unexplained exemption in lint rule is how rule erodes, so each one
named in `scripts/check-boundaries.sh` with its reason next to it.

**Why:** fourteen `json:"-"` tags were load-bearing security controls —
`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`. Two deleted
characters published password hash. This inverts default: field now
private unless DTO names it. Also makes `<slice>/http` mean something; with
tags on the model, the model still dictates the API and the adapter is
just a folder.

**Cost accepted:** ~40 mapper functions, now one set per slice rather than
one set per feature, and request DTOs had to split into a core `…Params`
type (no tags) plus an unexported wire type — otherwise the core would
import its own adapter.

## 10. `money.Money`, not `int64` beside a `Currency string`

**Why:** codebase hand-rolled it in two places — currency-consistency
loop in `order.PlaceOrder`, and two-field `Amount != … || Currency != …` compare
in `payment`'s verification path — across **twelve `Currency string` fields** that
could each drift from amount beside them. `Money` makes "amount without its
currency" unrepresentable, and collapses both hand-rolled checks into one
`ErrCurrencyMismatch` from `Add`/`Equal`.

Exactly **one** loose `Currency string` now survives outside adapter, and it
the deliberate exemption in §9: `internal/modules/payment/gateway/gateway.go`, external
gateway's own contract. `Money` maps down to its plain `int64`+`string` fields in
`payment/gateway/stripe` and `payment/gateway/midtrans`, which correct seam.

**Scope: four features — `order`, `payment`, `product`, `cart`.** Those
only ones whose data model carries currency at all. Two deliberately
excluded, and reasons load-bearing rather than bookkeeping:

- **`promotion` stays on `int64`.** `Promotion.Value` _polymorphic_: with
  `TypePercentage` it percentage
  (`internal/modules/promotion/domain/promotion.go:46` guards `value > 100`),
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

1. **`place.CouponReserver`** (`internal/modules/order/usecase/place/ports.go`) still passes `orderSubtotal int64`
   and returns `discountAmount int64`. Its implementer is `promotion`, which has no
   currency to honour `Money` with. Pairing happens on order's side of
   seam — `internal/modules/order/usecase/place/usecase.go` passes `subtotal.Amount` and rebuilds
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

**Why:** `go test ./...` runs every test binary concurrently against one
shared container per binary. Collapsing them into a single `test/integration`
package makes them sequential, and `t.Parallel()` cannot recover it because
subtests would share one database. `test/e2e` exists for checkout and
refund sagas — flows spanning five modules no single feature package can
own.

Now that every module is sliced, "next to their code" means next to the
*slice*:
`internal/modules/<feature>/usecase/<slice>/postgres/repository_test.go`
is its own package and its own test binary, and every slice's test package
under one module claims that module's database by the same name
(`test_order`, `test_payment`, …) rather than one name per package. They
never tear each other down — the database is created once, under an
advisory lock, and never dropped — but they also never get a clean table
between them, which is why `ResetDB` is off-limits to any of them (see
`ARCHITECTURE-LIMITATIONS.md`).

**Cost accepted:** 75 packages call `testhelper.MustStartPostgres` or
`MustStartRedis` today (`grep -rl testhelper.MustStart --include='*_test.go'
. | xargs -n1 dirname | sort -u | wc -l`) — up from 19 test binaries before
this phase's slicing, each still with its own `TestMain`, none of it
collapsed into fewer database claims: a module gained test binaries in the
same slicing that left it with one database name shared across them.

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
`payment/jobs.Dispatcher.Process` and `notification/jobs.Worker.Process`,
inside a slice package. Every other call, in every other slice, needed no
change to start carrying the context's attributes.

Two details are load-bearing rather than stylistic. `ContextHandler`
overrides `WithAttrs` and `WithGroup`, because the methods promoted from
the embedded handler return the *inner* handler — `logger.With(...)` would
otherwise produce a logger that silently emits no context attributes.
And `WithAttrs` clips the slice before appending, because two contexts
derived from one parent would otherwise share a backing array and
overwrite each other.

**The cost:** you can no longer read a single log call and know everything
it emits. `order/usecase/expire.UseCase.ExpireStale`'s
`c.logger.ErrorContext(ctx, "failed to expire order", slog.String("order_id", o.ID.String()), slog.String("error", err.Error()))`
also emits `runner`, because it runs inside the payment runner's per-tick
sweep, and nothing at that line names it. In exchange, 32 repeated
attributes are gone and `request_id` reaches code that has never heard of
HTTP.

## 13. A `<feature>/contract/` package publishes the structs that cross a boundary

Seven of fourteen modules — `auth cart inventory order payment product user` —
have a `contract/` package: `order/contract.Order`,
`payment/contract.ChargeRequest`, and their siblings. Each package
imports no module and no platform package, so importing one can never pull the
producer's implementation along with it — a consumer takes the type by value
and never learns how it is built. A port still names the type it needs; the
contract package supplies only the shape, never the interface — that stays
declared by the consumer, per decision 2.

**Why:** decision 2's trick — a producer's own value already had a method
named what the consumer's port asked for, so `promotion.Service`
satisfies `payment.CouponPort` with no adapter at all — works for scalars and for
interfaces a producer already implements. It does not work when what crosses
is a struct: two modules cannot each declare their own `User` and have the
compiler agree the two are the same type. Every module that names a struct
type in a port it does not own needed exactly one published type for that
struct; the other seven modules never pass one across a port and have no
`contract/` to show for it.

**Cost accepted:** a module now has a published surface. Before, changing an
internal struct's shape was a one-module diff; changing a published contract
type is now a change every consumer of that module must absorb, whether or
not the field they care about moved. The alternative this replaced — structurally-identical
types declared in both modules plus a mapping function between them — paid the
same cost at every call site instead of at the one file that changed;
`contract/` moves it from many places to one, but does not remove it. See
`ARCHITECTURE-LIMITATIONS.md` for where that published surface can go wrong on
its own account.

## 14. A module is a business boundary containing vertical slices

`internal/modules/shipping/` is the reference — first of fourteen modules
moved to this shape; `payment` was the last, and all fourteen now hold it.
It holds `domain/` (its aggregate
and rules — `Shipment`, `ShipmentStatus`, `CanShipOrder` — module-private by
convention, not by any check: nothing outside `shipping` needs it, so nothing
imports it), `module.go` (constructs the slices, imports no transport
package), a `contract/` **only if another module consumes a struct from it**
(`shipping` has none: only `internal/bootstrap` and `internal/transport`
import it at all, and both are the wiring layer, not a consumer), and a
`usecase/` directory holding one package per use case — `query`, `create`,
`updatetracking`, `deliver`.

**The `usecase/` level is load-bearing, not decoration.** It makes "is this a
slice?" a fact about the path rather than a judgement about the name: check 5
(`check_sibling_slice_imports`) used to carry a seven-name denylist —
`domain`, `contract`, `http`, `gateway`, `worker`, `postgres`, `redis` — that
had to be kept in step by hand, and a missing entry either reported a
legitimate shared directory or left a real slice unscanned. A directory under
`usecase/` is a slice, and that is the whole rule. The price is honest: the
denylist never listed `jobs`, so `payment/jobs` and `notification/jobs` were
scanned as slice-importers before and are not now. No such import exists
today, and the narrowing is recorded rather than described as a superset.

A slice owns its use case — one exported `UseCase` in one `usecase.go`,
whatever the slice does: `create.UseCase.Execute` writes,
`query.UseCase.GetByOrderIDForUser` reads. The per-role names this decision
first used (`Command` for a writer, `Reader` for a reader) are retired
everywhere, along with `Service`, `Store` and `Applier`. One name means a
reader can open any of the 66 slices and know what to look for, and it stops
the question "is a slice with two methods still a `Command`?" from having to
be answered at every boundary — `order/usecase/transition.UseCase` has ten.
A slice also owns its storage port, its `postgres/` adapter and its `http/`
adapter. "Own" is literal: `updatetracking.Repository` and
`deliver.Repository` both need `GetByID`; each declares it rather than
sharing one. A slice declares a port for anything it does not implement
itself — another module's capability or a sibling slice's — and `module.go`
wires it. Three of payment's four slices declare a port on order state, each
named for the role it needs: `charge` and `refund` each declare `OrderGetter`
(`GetSnapshot`) and `OrderItemsGetter` (`ListItemQuantities`) — identically
shaped, because both read the same order fields before acting — and each
also declares its own `OrderUpdater`, sized for what it alone writes:
`charge`'s carries five `Mark*` methods, `refund`'s one (`MarkRefunded`);
`webhook` declares a third, one-method `OrderUpdater` of its own
(`CancelUnpaid`), for a write neither sibling makes. `charge` and `refund`
naming their read ports the same shape the same way is the point: one name
per shape, one shape per name, so a reader comparing the two is comparing
roles rather than decoding whether two `OrderGetter`s mean the same thing —
`OrderUpdater` does not get that benefit here, since the same name covers
three unrelated shapes across three packages. `module.go`'s `Deps` folds
the read side into one port, not two: `OrderReader` (`GetSnapshot` and
`ListItemQuantities`, shared by `charge` and `refund`), wired to
`order.Module.Query` by name-match. The write side stays two ports, not
one, because `charge` and `refund`'s writes overlap enough for one
interface to cover both by superset while `webhook`'s does not:
`OrderTransition` (the union of all six `Mark*` methods `charge` and
`refund` need between them) wires to `order.Module.Transition`, and
`OrderCanceller` (`CancelUnpaid` alone) wires to `order.Module.Cancel` for
`webhook` only, same trick as decision 2. `query` declares none and
takes no `TxRunner` either, because both its methods only read its own
repository — there is nothing outside itself to ask.

**Why:** a use case's whole implementation is one directory, and what it
depends on is its own `ports.go`, or its absence. `ls
internal/modules/payment/usecase/` is the module's use-case list — nothing
else is in there to read past — and a slice with no `ports.go` reaches
nothing beyond itself; `query` is that case, not an oversight.

**Cost accepted:** three packages per slice (root, `postgres/`, `http/`), so
shipping alone went from 3 packages, layered, to 14 today, sliced —
`usecase/` is a directory, not a package, so it adds a path segment to every
import in the module without adding a package to compile. `module.go`
imports each slice's own package plus that slice's `postgres/` adapter, and
those import paths are one segment longer than they were. Every slice's
adapter is named `postgres` or `http`, same as every other slice's, so
importing it needs an alias, not merely permits one — five aliased imports in
shipping's `module.go` today. Response DTOs are duplicated across slices that
return the same shape: `query`, `create`, `updatetracking` and `deliver` each
declare their own unexported `shipmentResponse` rather than share one —
deliberately, so one endpoint's new field cannot appear in another's output.
The other thirteen modules paid the same multiplier as each was sliced in
turn, scaled to however many use cases each module turned out to need —
decision 3's package counts, and decision 9's response-DTO-duplication note,
are the tally now that all fourteen have paid it. Slicing's first bill was
paid with forwarding methods on `Module` — 20 of them, 8 already dead by the
time anyone counted. They are gone. The cost moved to the consumers' `Deps`:
`order.Deps` carries ten port fields where it carried six, and `place.New`
takes a `Deps` struct because twelve positional arguments is not readable.
In exchange every port names methods from exactly one slice, so a dead port
cannot hide behind a live one.

## 15. The transport owns every URL; a module owns none

Every route in the system is declared in `internal/transport/http/routes/`,
one file per feature, 14 files, 64 routes. Each file exports one function —
`routes.Cart(authed, m *cart.Service, v *validator.Validator)`,
`routes.Order(authed, admin, m *order.Module, v, writeLimiter)` — and
`router.go` calls all fourteen in one readable list. A module supplies a
handler with exported route methods and nothing else: no `routes.go`, no
`RegisterRoutes`, no `middleware.RouteGroup` in its signature, no string
beginning with `/`.

**This reverses what decision 9 first said.** That decision put a
`http/routes.go` at each feature root, holding `RouteDeps` and
`RegisterRoutes`, on the argument that a feature should arrive with its
routes attached. Four things went wrong with that, and none is stylistic:

- **The module became the router's peer instead of its supplier.** A feature
  root `http/` imported `internal/transport/http/middleware` to name a
  `RouteGroup`, so check 6 (`check_transport_direction`) needed a second
  exempt location on top of a slice's own `http/`. The arrow between the two
  trees pointed both ways, and a rule with two exemptions is a rule that
  argues with itself.
- **No one could read the API.** The 64 routes were spread over 14 files in
  14 different modules, each mounting handlers by a `RouteDeps` struct.
  Answering "what does `/api/admin/orders/{id}` do" meant knowing which
  module to open first.
- **Prefix drift went unnoticed.** A feature's `routes.go` named a path
  fragment and the router named a prefix, and nothing put the two halves in
  one place where a reader could see the whole URL.
- **A DTO could drift out of its slice.** With a json-tag-exempt `http/` at
  the feature root, a response type could migrate one level up out of the
  slice that serialises it and still pass check 1. Check 1's exempt location
  is now `usecase/<slice>/http/` alone, so that move fails.

**Why the transport, specifically:** a URL is a transport fact. Which verb,
which path, which middleware group, which rate limiter — none of it is a
decision `cart` is qualified to make, and all of it is a decision the person
adding a second transport would have to make again. Keeping it in one tree
means the day a `grpc/` arrives, the slices do not move: a slice grows a
`grpc/` sibling next to its `http/` and a second route tree names its
methods.

**Cost accepted, and it is real.** Adding a slice with a route now touches
**two trees**: the module for the handler, `internal/transport/http/routes/`
for the URL. That is one more file per feature than decision 9's shape
needed, and the two can be edited apart: a slice's `http/` package that no
route file mounts compiles clean, passes every check, and serves nothing,
with no `go build` failure to say so. **A feature is also no longer copy-pasteable with its routes
attached**: lifting `internal/modules/wishlist/` into another repository now
lifts a module that answers no URL until someone writes its route file by
hand. Under decision 9 that directory was self-contained. It is not any more,
and for a template — where copying a feature is a thing readers actually do —
that is the sharpest edge of this decision, not a footnote to it.

**What it does not cost:** no test moved. A slice's `handler_test.go` builds
its own `middleware.NewRouteGroup` and always did, so it never depended on
the feature route table it now cannot see. The flip side is that a handler
test still proves nothing about the URL — `internal/transport/http/router_test.go`
and `test/e2e/` are the only things that can, and neither snapshots the whole
table (see `ARCHITECTURE-LIMITATIONS.md`).

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

The rejection has a sharper answer now than "introduce it when a third
type needs it": `<feature>/contract/` is what a cross-module vocabulary
type belongs in instead, and it is scoped per producer rather than pooled
where anyone can reach it, not in a shared namespace every module imports.
Check 7 (`check_contract_leaf`)
is what keeps that answer from decaying into `internal/shared/` by another
name: a `contract/` package that started importing another module's
`contract/`, or its own `domain/`, would be `shared/` again with extra
steps, and the check refuses both. If a vocabulary type genuinely serves
no single producer — the case `internal/shared/` would have existed for —
that case has not appeared yet; introduce it then, and expect the same
scrutiny a `contract/` package gets.

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

**Rejected.** Exactly one module defines an address type — `Address`, in
`internal/modules/order/domain/order.go` now that `order`'s domain types
live in `domain/` rather than at the feature root — and `shipping` has
none — shipments key off `order_id`. Promoting a single-consumer type to a
shared package is the opposite of what "shared" should mean.

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
never fill. Add when there is proto. Decision 15 is what makes that cheap
when it happens: a slice grows a `grpc/` sibling next to its `http/`, and a
second route tree beside `internal/transport/http/routes/` names its
methods — no slice moves.

## `x/webhook/` as a package

**Rejected** in favour of
`internal/modules/payment/usecase/webhook/http/webhook_handler.go` — the
callback is `payment`'s own slice now, not a bolt-on file at the feature
root. Webhook _is_ HTTP; splitting it out fragments the handler across two
packages for one use case. Things that actually need protecting — no JWT
middleware, raw body access, signature verification — already handled by the
route group `internal/transport/http/routes/payment.go` mounts it on.

## `notification/worker/`

**Rejected.** Notification's `jobs/` package's `Worker` satisfies
`jobs.Processor` directly. Writing `worker.NewProcessor(w)` that returns `w`
to fill slot would be ceremony teaching opposite of decision 4.

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
