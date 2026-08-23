# Architecture decisions

Why this codebase shaped this way — including things it deliberately
does **not** do, because structure only teachable if roads not taken
visible.

Read `./ARCHITECTURE-LIMITATIONS.md` for bills these decisions
carry, and `./db/OWNERSHIP.md` for table ownership map — which
`make check-boundaries` parses, so enforced not merely asserted.

**Two of the seventeen decisions below are history, not current practice,
and both say so at the top of their body.** Decision 14 — a module is a
boundary containing vertical slices — is **reversed**; decision 16 records
what replaced it and what the reversal cost. Decision 15's rule still stands
and is still machine-checked, but the mechanism inside it was revised. Both
bodies are kept in the past tense rather than deleted: they are why the tree
looked the way it did for a year, and a decision record that shows only the
answer that survived teaches nothing about the ones that were tried.

---

## 0. This repository is a template, so the structure is the product

Every decision below judged by **what it teaches**, not by what cheapest to
maintain. Unusual, and changes calculus: where product codebase
would keep working shortcut, this one pays for boundary compiler can
enforce, because reader will copy whatever they find here into real system.

Two consequences worth naming, since they look like mistakes otherwise:

- The `adapter/postgres` / `adapter/http` split costs an import alias
  wherever an adapter is wired, and there are exactly two such files.
  `internal/bootstrap/app.go` carries 14 (thirteen `*pg` plus `userredis`);
  `internal/server/routes.go` carries 15, one per module that serves a route.
  In a product codebase, hard to justify. Here it is the point: a physical
  boundary teaches the port/adapter distinction in a way a file-naming
  convention cannot.
- Where rule exists, machine-checked (`make check-boundaries`). Rule
  living only in README rots, and template shipping rotted rule
  teaches rot.

Backward compatibility explicitly **not** goal. API shapes changed
freely where better design demanded.

---

## 1. Feature modules, not layers

`internal/modules/order/` holds everything order owns — not
`internal/domain/order` plus `internal/application/order` plus
`internal/infrastructure/order`. This decision is about that outer boundary
only. Decision 16 covers what sits inside it: one `service.go`, one
`repository.go`, a `domain/` and an `adapter/`. Decision 14 held the middle
of the story, when the inside was a `usecase/` tree of one package per use
case; it is reversed, and the flat shape this decision first described is
what came back.

**Why:** `payment.New()` and `payment.Repository` read
naturally in Go; `application.NewService()` and `domain.Repository` put the
layer name in every import and tell nothing about what the code is for.
Layered trees also scatter one change across three directories.

**Cost accepted:** a module is a directory tree, not a file — a `domain/` and
an `adapter/` below the root package, and one directory deeper than the flat
feature package this decision first proposed, because of the `modules/`
wrapper below.

**Why the `modules/` wrapper:** 16 directories sit under `internal/modules/`,
one below their old home, so `scripts/check-boundaries.sh` reads the list
straight off the filesystem instead of maintaining a denylist of everything
under `internal/` that is _not_ a module. The denylist it replaced had
already drifted once — `money` was missing from it, so a shared value object
was briefly treated as a module subject to ownership checks. `money` lives
under `internal/modules/` on purpose now, and check 4's root-package rule is
what makes that safe: any module may name `money.Money`, and nothing may
reach past it. A directory is right by construction in a way a list of
exceptions cannot be.

## 2. Ports live with the consumer

The consumer is a module. `internal/modules/order/ports.go` declares
`CartLocker`, `CartReader` and `CartClearer` — the three interfaces `order`
alone needs from cart. `cart` publishes none of them; `order` names exactly
what it needs and something else satisfies it. Nine modules have a
`ports.go`; the other seven reach nothing outside themselves and have none.
Every port file, in every module, is called `ports.go` — naming one after the
dependency instead, which this decision once allowed, happens nowhere in the
tree.

**Why:** no module imports another's implementation, so the dependency graph
has no cycles by construction and each module's port list is exactly the API
it would need if extracted. It pays off immediately: because interfaces are
declared narrow at the consumer, `promotion.Service` satisfies both
`order.CouponReserver` and `payment.CouponReleaser` directly, and
`notification/jobs.Worker` satisfies `platform/jobs.Processor` directly — no
adapter ever needed writing.

**Cost accepted:** none, where a producer's own method already matches what
the consumer's port asks for; that is free to declare. Where what crosses is
a struct rather than something a `Service` already satisfies by name, decision
13 (`contract.go`) is what pays for it, and what it pays is a published
surface: adding a field to one is a cross-module change, not a local one.

There is a second cost, but it is latent, not live. When a consumer declares
three narrow ports over one producer — `shipping`'s `OrderGetter`,
`OrderShipper` and `OrderDeliverer` — all three are satisfied by the same
`*order.Service` value, so there is no wrong value to paste into a `Deps`
field today: swap two field names and either nothing changes, or Go refuses
the duplicate field. Under decision 14, `OrderShipper` and `OrderDeliverer`
already shared one slice value; only `OrderGetter` differed, and pasting it
over either sibling's field was a real compile error then. Narrow consumer
ports are still the right shape; what is gone is the cross-check between a
field and its value, and it is missed only the day this consumer needs two
ports backed by two different values and someone wires the wrong one.
`ARCHITECTURE-LIMITATIONS.md` counts what is exposed.

## 3. Adapters are subpackages named for their technology

`internal/modules/order/adapter/postgres`,
`internal/modules/order/adapter/http`, `internal/modules/user/adapter/redis`,
`internal/modules/payment/adapter/jobs`. One directory level — `adapter/` —
groups them, so `ls internal/modules/order/` shows the module's own surface
without an adapter in the way.
`payment/gateway/stripe`, `payment/gateway/midtrans` and `payment/gateway/mock`
are the exception that proves the rule at a different scope: an adapter family
for one outbound port, still named for its technology, just not under
`adapter/` — because `gateway.Gateway` is a port the module *calls* rather
than one that answers a caller.

**Why:** the dependency rule becomes a compile error, not a convention — a
module cannot import its own `adapter/postgres` without a cycle, so SQL
physically cannot leak into `service.go`.

**Cost accepted:** 15 packages named `postgres` under `internal/modules`
today, 15 named `http` and one named `redis` — re-run `find internal/modules
-type d -name postgres | wc -l` (and `http`, `redis`) rather than trust these.
Thirteen of the fifteen `postgres` packages are a module's own
`adapter/postgres`; the other two are the job queues'
(`notification/jobs/postgres`, `payment/jobs/postgres`), which decision 16
leaves outside `adapter/` because they back a queue rather than the module's
aggregate. Every adapter package is named for its technology, same as every
other module's, so importing one needs an alias rather than merely permitting
one — and both places that import them are single files: 14 aliases in
`internal/bootstrap/app.go`, 15 in `internal/server/routes.go`. The cost is
concentrated in two files for the whole binary, deliberately: adding a module
touches each of them once.

## 4. Adapter subpackages exist only where adaptation is needed

`auth` has **no** store anywhere in the module — the one thing it needs from
storage it asks `user` for, through a single port (`auth.UserDirectory`), and
it keeps nothing of its own. `checkout` has neither a store nor a `domain/`:
it orchestrates two other modules and owns no state. `money` has no adapter
at all, no `Service` and no store; it is a value object. `user` is the one
module in the repo with two backing stores, `adapter/postgres` and
`adapter/redis` — every other module has at most one. Seven modules have no
`ports.go`, because nothing they do reaches outside the module.
`notification` has **no** `worker/` package because its `jobs.Worker`
satisfies `platform/jobs.Processor` directly — one value doing both roles —
and `payment` needs even less: `Claim` and `Prune` are methods on
`*payment.Service` itself, with no `Queue` type standing between them and the
value `cmd/worker` hands `jobs.Runner`.

`contract.go` is not counted as an adapter — it adapts no technology,
decision 13 covers it on its own terms, and a module gets one independently
of how many adapters it needs.

**Why:** a pass-through package created to make the tree look uniform teaches
that adapters are bureaucracy. Absence is the lesson. It reads more clearly
now than it did under decision 14: `auth` having no store is one absent
directory, where before it was four slices that each happened to have none.

**Cost accepted:** you cannot predict a module's shape without looking.

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

## 9. `adapter/http` owns the wire format

No `json` tag exists on a type **this system owns** outside
`internal/modules/<feature>/adapter/http/`. Every endpoint owns its request
DTO, its response DTO and an explicit mapping between them and the domain:
59 unexported `…Request`/`…Response` structs and 28 `to…Response` mappers live
in those fifteen packages, beside the handler that serialises them. Files
inside `adapter/http` split by **handler role**, not one per endpoint:
`handler.go` for the default handler, `admin_handler.go` where the module's
routes split by caller role, and `webhook_handler.go` in
`payment/adapter/http`, whose only public route is the gateway callback and
which therefore has no `handler.go` at all. Each has a `_test.go` beside it,
`package http`, holding both route-level tests and tests that reach unexported
mappers directly. **No module has a root `http/`, and none names a URL**: the
`routes.go` that used to sit at a feature root is gone, and every URL lives in
`internal/server/routes.go` — decision 15.

**Response DTOs are duplicated across modules on purpose.** `order` and
`checkout` each declare their own `orderResponse`, `orderItemResponse` and
`addressResponse` rather than share one, even though `order`'s and
`checkout`'s are structurally identical today. Someone will read that as an
oversight and try to collapse it into one shared type; that is the one thing
not to do here. The reason is the same one this decision exists for: one
endpoint's new field must not be able to appear in another endpoint's response
by sharing its struct. Inside one module the duplication is gone — merging the
slices merged the DTOs that had duplicated per slice — so what remains is
duplication across a module boundary, which is exactly where it earns its
keep.

`make check-boundaries` enforces the tag rule, not the file layout. Nothing
checks how handlers are distributed across files. What the script checks is
`json` tags outside a module's `adapter/http` (check 1), cross-module table
references in SQL (check 3), and a module importing anything from another
module beyond its root package (check 4).

Two exemptions, both deliberate and both allowlisted by name in the check:

- **`internal/modules/payment/gateway/gateway.go`** — `ChargeRequest`/`ChargeResponse`/`RefundRequest`/
  `RefundResponse` are the _external_ gateway's wire contract, not ours. Those tags
  describe someone else's API, and `payment/gateway/stripe` and `payment/gateway/midtrans`
  marshal them on the way out. Mapping `Money` down to their plain `int64`+`string`
  fields in those adapters is a correct seam, not a leak.
- **`internal/server/response/response.go`** — the shared envelope every
  handler in every module writes through: transport infrastructure, not a
  domain model, the same role `internal/platform/paging/`'s cursor/offset
  envelope plays one layer down.

An unexplained exemption in a lint rule is how the rule erodes, so each one is
named in `scripts/check-boundaries.sh` with its reason next to it. Drop check
1's `adapter/http` arm and it reports 295 tags in fifteen adapters at once,
which is what makes the arm load-bearing rather than decorative.

**Why:** fourteen `json:"-"` tags were load-bearing security controls —
`user.PasswordHash`, `payment.GatewayResponse`, `order.RequestHash`. Two
deleted characters published a password hash. This inverts the default: a
field is now private unless a DTO names it. There are zero `json:"-"` tags
under `internal/` today and check 1b keeps it that way. It also makes
`adapter/http` mean something; with tags on the model, the model still
dictates the API and the adapter is just a folder.

**Cost accepted:** 28 mapper functions, and request DTOs had to split into a
core `…Params` type (no tags) plus an unexported wire type — otherwise the
core would import its own adapter. And the failure mode that replaced
`json:"-"` is naming the *wrong* DTO: `user/adapter/http` holds
`toUserResponse` (five fields) and `toAdminUserResponse` (nine) in one
package, and swapping them in the public handler compiles.
`ARCHITECTURE-LIMITATIONS.md` prices that.

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

1. **`order.CouponReserver`** (`internal/modules/order/ports.go`) still passes `orderSubtotal int64`
   and returns `discountAmount int64`. Its implementer is `promotion`, which has no
   currency to honour `Money` with. Pairing happens on order's side of
   seam — `order.Service.Place` passes `subtotal.Amount` and rebuilds
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

"Next to their code" means next to the adapter:
`internal/modules/<feature>/adapter/postgres/repository_test.go` is its own
package and its own test binary, and it claims its module's database by name
(`test_order`, `test_payment`, …). Two modules put two test packages on one
name — `notification` and `payment` each have a `jobs/postgres` beside their
`adapter/postgres` — and those two never tear each other down, because the
database is created once under an advisory lock and never dropped. They also
never get a clean table between them, which is why `ResetDB` is off-limits to
any package inside a module (see `ARCHITECTURE-LIMITATIONS.md`).

**Cost accepted:** 25 packages call `testutil.MustStartPostgres` or
`MustStartRedis` today (`grep -rl testutil.MustStart --include='*_test.go' .
| xargs -n1 dirname | sort -u | wc -l`), each with its own `TestMain`. That is
down from **75** at the last commit before the flatten (`git grep -l
testhelper.MustStart 0ee2cc5 -- '*_test.go' | xargs -n1 dirname | sort -u |
wc -l`), because a sliced module gained a test binary per slice while still
sharing one database name across all of them. One `service_test.go` plus one
`adapter/postgres` test package per module is what the flatten leaves. Neither
number was ever the point — the point is that they run concurrently, one
container per binary, and collapsing them into a single package would
serialise the suite.

## 12. Log attributes travel in the context, not in signatures

A service that logs `request_id` has no business knowing what an HTTP
request is. The alternative — threading the value down as a parameter, or
handing every layer a pre-built logger — makes a transport concern part of
fifteen service APIs.

So `logger.WithAttrs(ctx, ...)` stores attributes in the context and
`logger.ContextHandler` merges them into every record. `logger.Setup`
installs the wrapper, so every logger in both binaries has it. Services
keep their constructor-injected `*slog.Logger` and their existing
`InfoContext(ctx, ...)` calls unchanged. Only the four edges that name an
attribute carry a `logger.WithAttrs` line: `middleware.RequestID`,
`middleware.Auth`, `jobs.Runner.Start`, and each queue-draining `Process`
(`payment/adapter/jobs.Dispatcher.Process` and
`notification/jobs.Worker.Process`). Every other call in every other module
needed no change to start carrying the context's attributes.

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

## 13. `contract.go` publishes the structs that cross a boundary

Seven of the sixteen module directories — `auth cart inventory order payment
product user` — have a `contract.go`: `order.Snapshot`, `user.Profile`,
`payment.ChargeRequest`, and their siblings. A port names the type it needs;
`contract.go` supplies only the shape, never the interface — that stays
declared by the consumer, per decision 2.

**Why:** decision 2's trick — a producer's own value already has a method
named what the consumer's port asks for, so `promotion.Service` satisfies
`payment.CouponReleaser` with no adapter at all — works for scalars and for
interfaces a producer already implements. It does not work when what crosses
is a struct: two modules cannot each declare their own `User` and have the
compiler agree the two are the same type. Every module that names a struct
type in a port it does not own needs exactly one published type for that
struct; the other nine directories never pass one and have nothing to show.

**Why a file and not a package.** This decision used to name a `contract/`
package, and check 7 (`check_contract_leaf`) held every one of them to
stdlib, `github.com/google/uuid` and `money` only — so importing a module's
published types could never drag its `domain/` along. When decision 16 made
each module's root package the published surface, `contract/` had nowhere left
to be: a separate package for types that already live in the importable
package is a level of indirection with nothing on the other side of it. So the
types moved into `contract.go`, in the root package, and check 7 was retired
because it had nothing left to be true of.

**Cost accepted, and it grew.** A module has a published surface: changing an
internal struct's shape used to be a one-module diff, and changing a published
contract type is a change every consumer must absorb. That much is unchanged.
What the move to `contract.go` added is that the guarantee is gone.
`inventory.StockState` is the worked example: while it lived in
`inventory/contract`, `order` and `payment` imported a package that provably
held nothing but the type. They now import `inventory`, which imports
`inventory/domain` and declares `Service` and `Repository`. The type is the
same; the import is not. Nothing machine-checks what a `contract.go` may
carry, and nothing stops a consumer that imported the module for one struct
from calling a method on its `Service`. See `ARCHITECTURE-LIMITATIONS.md`.

## 14. A module is a business boundary containing vertical slices

> **REVERSED.** Everything below is in the past tense and describes no part of
> the current tree: there is no `usecase/` directory anywhere, and `find
> internal -type d -name usecase` prints nothing. Decision 16 replaced this
> one and records what the reversal cost. It is kept because it is why the
> tree held 226 packages for a year, and because the argument for slicing is a
> real argument someone will make again.

`internal/modules/shipping/` was the reference — the first of fourteen modules
moved to this shape, and `payment` was the last. A module held `domain/` (its
aggregate and rules — `Shipment`, `ShipmentStatus`, `CanShipOrder` —
module-private by convention, not by any check), `module.go` (constructing the
slices, importing no transport package), a `contract/` package **only if
another module consumed a struct from it**, and a `usecase/` directory holding
one package per use case: for shipping, `query`, `create`, `updatetracking`,
`deliver`.

**The `usecase/` level was load-bearing, not decoration.** It made "is this a
slice?" a fact about the path rather than a judgement about the name. Check 5
(`check_sibling_slice_imports`) had carried a seven-name denylist — `domain`,
`contract`, `http`, `gateway`, `worker`, `postgres`, `redis` — kept in step by
hand, and a missing entry either reported a legitimate shared directory or
left a real slice unscanned. A directory under `usecase/` was a slice, and
that was the whole rule.

A slice owned its use case — one exported `UseCase` in one `usecase.go`,
whatever the slice did. `create.UseCase.Execute` wrote,
`query.UseCase.GetByOrderIDForUser` read. One name meant a reader could open
any slice and know what to look for, and it stopped the question "is a slice
with two methods still a `Command`?" from having to be answered at every
boundary. A slice also owned its storage port, its `postgres/` adapter and its
`http/` adapter, and "own" was literal: `updatetracking.Repository` and
`deliver.Repository` both needed `GetByID`, and each declared it rather than
sharing one. A slice declared a port for anything it did not implement itself
— another module's capability or a sibling slice's — and `module.go` wired it.

**Why it was worth doing:** a use case's whole implementation was one
directory, and what it depended on was its own `ports.go`, or its absence.
`ls internal/modules/payment/usecase/` was the module's use-case list, with
nothing else in there to read past, and a slice with no `ports.go` provably
reached nothing beyond itself. That is a real property, and it is the one
decision 16 gave up.

**Why it was reversed — the cost, measured.** Three packages per slice (root,
`postgres/`, `http/`): shipping alone went from 3 packages, layered, to 14,
sliced. Response DTOs duplicated across every slice returning the same shape.
An import alias per slice adapter in `module.go`, because every slice's
adapter was named `postgres` or `http` like every other slice's. The same
interface declared four times inside one module because no slice could import
a sibling — `TransitionApplier` in `order`'s `cancel`, `expire`, `place` and
`recoverstale`; `ProductLookup` three times in `cart`; `StatusInvalidator`
three times in `user`. Slicing's first bill was paid with forwarding methods
on `Module`, several of them dead by the time anyone counted; deleting those
moved the cost onto the consumers' `Deps`, which grew a field per capability.
Across the fourteen modules it came to **226 Go packages and 655 Go files**
under `internal/modules`, measured at the last commit before the flatten
(`git ls-tree -r --name-only 0ee2cc5 -- internal/modules`).

And the boundary all of that priced was never enforced anywhere it mattered.
Check 5 refused a slice importing a sibling slice, so two use cases in one
module — same team, same deployment, same database — talked through a port, a
mock and a `Deps` field. The machinery existed; the boundary it modelled did
not.

## 15. The transport owns every URL; a module owns none

> **The rule stands and is machine-checked (check 6). The mechanism inside it
> was revised.** Where this decision put fourteen files each exporting one
> function, there is now one `registerRoutes` function in
> `internal/server/routes.go`. The four bullets below are still the reasoning
> for why routes left the modules; only the shape of the destination changed.
> Decision 16 says why it collapsed.

Every route in the system is declared in `internal/server/routes.go` — one
unexported `registerRoutes` function, 64 routes, fifteen labelled blocks —
which `NewRouter` in `internal/server/server.go` calls once, handing it the
four route groups and the order-write rate limiter. A module supplies a
handler with exported route methods and nothing else: no `routes.go`, no
`RegisterRoutes`, no `middleware.RouteGroup` in its signature, no string
beginning with `/`.

**This reversed what decision 9 first said.** That decision put a
`http/routes.go` at each feature root, holding `RouteDeps` and
`RegisterRoutes`, on the argument that a feature should arrive with its routes
attached. Four things went wrong with that, and none is stylistic:

- **The module became the router's peer instead of its supplier.** A feature
  root `http/` imported the transport's `middleware` package to name a
  `RouteGroup`, so check 6 (`check_transport_direction`) needed a second
  exempt location on top of the module's own handler package. The arrow
  between the two trees pointed both ways, and a rule with two exemptions is a
  rule that argues with itself. Check 6 has exactly one exempt location today:
  `internal/modules/<feature>/adapter/http/`.
- **No one could read the API.** The 64 routes were spread over 14 files in 14
  different modules, each mounting handlers by a `RouteDeps` struct. Answering
  "what does `/api/admin/orders/{id}` do" meant knowing which module to open
  first.
- **Prefix drift went unnoticed.** A feature's `routes.go` named a path
  fragment and the router named a prefix, and nothing put the two halves in
  one place where a reader could see the whole URL.
- **A DTO could drift out of the package that serialises it.** With a
  json-tag-exempt `http/` at the feature root, a response type could migrate
  one level up and still pass check 1. Check 1's exempt location is
  `adapter/http` alone, so that move fails.

**Why the transport, specifically:** a URL is a transport fact. Which verb,
which path, which middleware group, which rate limiter — none of it is a
decision `cart` is qualified to make, and all of it is a decision the person
adding a second transport would have to make again. Keeping it in one tree
means the day a `grpc/` arrives, no module moves: a module grows an
`adapter/grpc` beside its `adapter/http`, and a second route file beside
`internal/server/routes.go` names its methods.

**Cost accepted, and it is real.** Adding a route touches **two trees**: the
module for the handler, `internal/server/routes.go` for the URL. The two can
be edited apart — an `adapter/http` method that no route mounts compiles
clean, passes every check, and serves nothing, with no `go build` failure to
say so. **A module is also no longer copy-pasteable with its routes
attached**: lifting `internal/modules/wishlist/` into another repository lifts
a module that answers no URL until someone writes its routes by hand. Under
decision 9 that directory was self-contained. For a template — where copying a
feature is a thing readers actually do — that is the sharpest edge of this
decision, not a footnote to it.

**What it does not cost:** no test moved. A handler test builds its own
`middleware.NewRouteGroup` and always did, so it never depended on a route
table it now cannot see. The flip side is that a handler test still proves
nothing about the URL. What closed half of that gap is
`internal/server/routes_snapshot_test.go`: `TestRouteSnapshot` reads
`internal/server/testdata/routes.golden` — 64 lines of
`method<TAB>path<TAB>group` — and probes every one against the real
`NewRouter`, which is the test this decision made cheap by putting all 64
routes in one place. What it still cannot see is in
`ARCHITECTURE-LIMITATIONS.md`.

## 16. A module is one flat package with an `adapter/` directory

`internal/modules/order/` holds one `service.go`, one `repository.go`, one
`ports.go`, one `contract.go`, a `domain/`, and an `adapter/` with `postgres`
and `http` under it. One exported `Service` carries every method the module
offers. This reverses decision 14 and restores the shape decision 1
originally described, one directory deeper.

**Why:** three reasons, in the order they mattered.

- **The slice boundary was never enforced where it counted, and its cost
  always was.** Check 5 refused a slice importing a sibling slice, so a use
  case needing a sibling's capability declared a port and let `module.go` wire
  it — inside one module, between two packages the same team owned, across a
  line no deployment, database or ownership boundary followed. The port, the
  mock, the `Deps` field and the wiring were all real. The boundary was not.
- **The counts were the argument.** 226 Go packages and 655 Go files under
  `internal/modules` became **67 and 217**. 66 slices became 15 `Service`s. 26
  `ports.go` files became 9. Fourteen route files and a `router.go` became one
  `routes.go` plus a `NewRouter` in `server.go`. Every one of those is a
  directory or a file someone has to open to answer a question.
- **Duplication that served nothing.** `updatetracking.Repository` and
  `deliver.Repository` both declared `GetByID` because neither could import
  the other; one `shipping.Repository` declares it once. `TransitionApplier`
  was declared four times inside `order`, `ProductLookup` three times inside
  `cart`, `StatusInvalidator` three times inside `user`.

**Cost accepted, and this is the part to read before copying it.** Every item
here is a guarantee the sliced tree had and this one does not:

- **Module privacy stopped being a compile error.** Check 4 makes a module's
  root package importable, so `payment` can call `order.Place` — any exported
  method on any sibling `Service` — with nothing to stop it and no check able
  to tell a legal import from an illegal call. Under decision 14 the root
  package was off-limits and `contract/` was the only door. This is the single
  largest thing the flatten gave up, and
  `ARCHITECTURE-LIMITATIONS.md` leads with it.
- **`contract/` lost its leaf guarantee.** Published types moved into
  `contract.go` in each module's root package, which imports `domain/` by
  design, so check 7 (`check_contract_leaf`) had nothing left to be true of
  and was retired. Importing a module for one published struct now pulls its
  whole root package. Decision 13 records the detail.
- **Consumer ports collapsed onto one value, so the compiler stopped
  cross-checking a `Deps` field against the value assigned to it.** Ten of
  `order`'s port fields across four consumers now take the same
  `*order.Service`, and every one of them already holds the value it should —
  there is no swap to make until a consumer needs two of its fields backed by
  different producers again. Under decision 14 most of those fields did, and
  pasting one value into the other's field was a real compile error then.
  `ARCHITECTURE-LIMITATIONS.md` counts the pairs.
- **One module of sixteen keeps a weaker boundary rule.** `checkout` alone may
  import a module's `domain/` -- any of the fifteen, since the grant names the
  importer and not the target -- because `order.Service.Place`'s signature names
  `orderdomain.NewOrder` and `*orderdomain.Order` and `order/contract.go`
  publishes neither. Removing that exemption reports 7 violations, not zero.
  Closing it means moving both types into `order`'s published surface, which
  this decision did not do.
- **A public and an admin response mapper now sit in one package**, one
  identifier apart, in a codebase that controls field exposure by DTO
  omission rather than by `json:"-"`. Decision 9 names the example.

**What it did not cost:** the outer boundary. Decisions 1, 6, 7, 8 and 15 are
untouched — a module still owns its tables, still declares its own ports in
its own package, and still names no URL. Decision 4 reads better than it did:
`auth` having no store is more legible as one absent `adapter/postgres` than
as four slices that each happened to have none.

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
`internal/modules/money`, and `apperror` stayed where it was — moving it would have
touched nearly every file for nothing.

The rejection has a sharper answer than "introduce it when a third type
needs it": a producing module's own `contract.go` is where a cross-module
vocabulary type belongs, scoped to one publisher rather than pooled in a
namespace every module imports. Check 7 (`check_contract_leaf`) used to be
what kept that answer from decaying into `internal/shared/` by another name —
a `contract/` package importing another module's `contract/`, or its own
`domain/`, would have been `shared/` again with extra steps, and the check
refused both. Decision 16 retired it: `contract.go` sits in the root package,
which imports `domain/` by design, so **nothing machine-checks what a
`contract.go` may carry now.** That is the version of this rejection worth
watching: `money` is one directory under `internal/modules/` and any module
may import it, so a second value object landing beside it starts to look like
`shared/` with a different parent. If a vocabulary type genuinely serves no
single producer — the case `internal/shared/` would have existed for — that
case has still not appeared; introduce it then, and argue for it as loudly as
`money` was argued for.

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
when it happens: a module grows an `adapter/grpc` beside its `adapter/http`,
and a second route file beside `internal/server/routes.go` names its
methods — no module moves.

## `x/webhook/` as a package

**Rejected** in favour of
`internal/modules/payment/adapter/http/webhook_handler.go` — the
callback lives in `payment`'s own adapter now, not a bolt-on file at the
feature root. Webhook _is_ HTTP; splitting it out fragments the handler across two
packages for one use case. Things that actually need protecting — no JWT
middleware, raw body access, signature verification — already handled by the
route group `internal/server/routes.go` mounts it on.

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
