# Limitations this architecture creates

`ARCHITECTURE.md` record nineteen decisions and fifteen things this codebase deliberately not do. Every one bought something and charged for it. This file the invoice.

Exist because this repo a **template**: structure is product, someone about to copy into real system. Doc listing only what design make easy teach nothing — design never in danger of blame there. Reader need list of moments where they hit wall, so they recognize wall as this design's, not own mistake.

Each section state limitation, moment you meet it, what you must do about it. Where limitation is decision's shadow, decision cited not restated — read together.

Last section, [When not to copy this](#when-not-to-copy-this), for someone still deciding.

The first six sections are the bill for decision 16 — collapsing each module
from a tree of vertical slices into one flat package. They come first because
they are the newest, the least obvious, and the ones a reader copying this
template is most likely to inherit without noticing.

---

## A module's whole exported surface is reachable from every other module

**Where you hit it:** you add a method to `order.Service` so `checkout` can
call it, and discover `payment`, `shipping` and `review` can call it too — and
that nothing will ever tell you if one of them does.

Check 4 makes a module's root package importable. That is decision 16's
deliberate choice: the root package is the published surface, so
`order.Snapshot` and `promotion.Service`'s methods cross a boundary with no
separate `contract/` package standing between them. The cost is that
"published surface" now means *every exported identifier in the package*.
`payment` imports `order` for `order.Snapshot`, and having imported it can
call `order.Service.Place`, `order.Service.Apply`, `order.Service.ExpireStale`
— anything — with no port declared for it and no check able to object. Check 4
sees a legal root-package import, and whether the code then calls a method
some `ports.go` declares is not a fact about imports.

Under decision 14 this was a compile error. A module's root package was
off-limits and `<feature>/contract/` was the only door, so reaching a
sibling's behaviour meant declaring a port and having `bootstrap` wire it.
It is now a convention, and it is the largest single guarantee the flatten
gave up.

**Why it holds today.** Every cross-module call goes through a port declared
in the caller's own `ports.go` — nine files, 29 interfaces — and
`internal/bootstrap/app.go` is the only place any of them is wired. Those nine
files are still an honest dependency inventory. Nothing enforces that they
stay complete.

**What you would do.** Three options, ascending cost:

- **Keep the convention and review for it.** For each cross-module import,
  check that every identifier it reaches is named by a port in the importing
  package's own `ports.go`. That is what happens today, by hand.
- **Narrow the surface.** Unexport what no port names. Impossible for
  `Service` itself — `adapter/http` and `bootstrap` both need it — but it
  works for anything that drifted out of `service.go` and stayed exported out
  of habit.
- **Check the calls, not the imports.** A real fix parses Go: for each
  cross-module import, resolve the selector expressions and assert each names
  something a port in the importing package declares. That is a `go/types`
  pass, not a grep, and it is the only version of this check that could work.
  Nothing here does it.

## `checkout` is held to a weaker boundary rule than its fifteen siblings

**Where you hit it:** you read `scripts/check-boundaries.sh`, find a `case`
guarded by `if [ "$file_feature" = "checkout" ]`, and wonder whether it is a
leftover.

It is not. Every other module may import only another module's root package;
`checkout` may also reach a `domain/`. Read the grant as written: it is keyed on
the importer, not on the target, so `checkout` may import **any** of the fifteen
other modules' `domain/` packages and the check stays green -- add
`payment/domain` to `checkout/service.go` and `make check-boundaries` still
prints "Boundaries OK". `order/domain` is the only one it needs, and one
signature is the reason:

```go
func (s *Service) Place(
	ctx context.Context,
	userID uuid.UUID,
	in domain.NewOrder,
	idempotencyKey string,
) (*domain.Order, bool, error)
```

`order.Service.Place` takes `orderdomain.NewOrder` and returns
`*orderdomain.Order`, and `order/contract.go` publishes neither type. So
`checkout.Orders` cannot name what it has to name, and neither can
`checkout/adapter/http/handler.go`, which builds the input and serialises the
result. Removing the exemption reports **7** violations — `checkout/service.go`,
`checkout/ports.go`, `checkout/adapter/http/handler.go` and four test files —
not zero.

**The middle return value is the whole replay contract, and there is no other
signal to read it from.** A repeated `idempotencyKey` makes `Place` return the
previously-stored order with `created=false`; a fresh key returns the new
order with `created=true`. The order itself looks identical either way — same
fields, same status, same items — so any caller that needs to tell "this
request built a new order" from "this request replayed one" has to branch on
`created` directly. `checkout.Service.PlaceOrder` does exactly that: it only calls
`payments.Charge` when `created` is `true` and the order carries a balance,
precisely because inferring a replay from the order's status would have
nothing to distinguish a fresh `awaiting_payment` order from a replayed one
in that same status — both look identical on the wire. Any future caller with
replay-must-not-repeat logic of its own has to branch on the bool the same
way — there is nothing else on the returned `*domain.Order` that says which
call created it.

The grant is pinned on one axis and open on the other. It covers `domain/`
only -- never an adapter -- and `scripts/boundaries_test.go` holds it there from
both sides: one subtest asserts `checkout` importing `order/adapter/postgres` is
still reported, another asserts the `order/domain` import stays clean. Nothing
holds it to `order`'s `domain/` in particular: a per-target grant would have to
name the target, and this one names only the importer. So the grant's silent
failure mode — `NewOrder` and `Order` moving into `order`'s published surface
and leaving the exemption behind as an unnoticed permanent weakening — has
something to break.

**What it costs.** `checkout` sees `order`'s rich model: every field, every
method, every invariant, where its siblings see only what `contract.go`
publishes. Rename a domain field on `order` and `checkout` breaks; no other
module can.

**What you would do:** move `NewOrder` and `Order` into `order`'s published
surface — either as `contract.go` types with an explicit mapping inside
`order.Service.Place`, or by giving `Place` a signature written in published
types and keeping the domain construction in `order`. That is a design change
this refactor did not make, and it is not free: `Order` is the aggregate, so
publishing it means either duplicating the struct or publishing the model. Do
not describe this exemption as pending cleanup unless you are proposing one of
those two.

## One flat `Service` satisfying several of a consumer's ports leaves the compiler nothing to check

> **Describes the tree before decision 17, and before positional parameters.**
> Every `Deps` struct below has since collapsed to one field per producer, so
> the "two fields, one value" case this entry counts cannot occur any more.
> The `Deps` struct itself is gone one step further than that: every module's
> `New` now takes positional parameters, so there is no struct literal left
> for a field name to be pasted over in the first place. The 18 pairs and the
> ten-port-fields figure are kept as a record of what decision 17 closed; the
> "Decision 17 spent the same fact a different way" paragraph after the table
> says what replaced it.

**Where you hit it:** nowhere yet. A consumer declares one narrow port per
capability (decision 2) and the producer is one `Service` that satisfies all of
them, so the ports differ in shape but not in the value assigned and the
compiler has nothing left to check. You hit it the first time one consumer
needs two ports backed by two *different* values and someone assigns the wrong
value to one of them: that builds now, and under decision 14 it did not.
Counting one pair for every two fields on the same consumer that take the same
value, `internal/bootstrap/app.go`'s struct literals held **18** such pairs
before decision 17:

| Consumer        | Value        | Fields | Pairs |
| --------------- | ------------ | -----: | ----: |
| `order.Deps`    | `cartMod`    |      3 |     3 |
| `order.Deps`    | `inv`        |      3 |     3 |
| `payment.Deps`  | `ordMod`     |      3 |     3 |
| `payment.Deps`  | `inv`        |      2 |     1 |
| `checkout.Deps` | `ordMod`     |      3 |     3 |
| `checkout.Deps` | `paymentMod` |      2 |     1 |
| `shipping.Deps` | `ordMod`     |      3 |     3 |
| `product.Deps`  | `inv`        |      2 |     1 |

**Every pair assigns the same value to both fields**, so there is no live swap
to make. Paste `OrderShip: ordMod` over `OrderDeliver: ordMod` in `app.go` and
Go refuses it — `duplicate field name OrderShip in struct literal`. Delete the
`OrderDeliver` line instead of duplicating it and you have a *dropped* field,
not a swap: a struct literal compiles with a field left unset, the result is a
`nil` port, and nothing caught that before the flatten either — see
[RESOLVED: a dropped constructor argument used to be silently
`nil`](#resolved-a-dropped-constructor-argument-used-to-be-silently-nil).

So the flatten introduced no bug. It removed a guarantee, and the guarantee
was real. Under decision 14 most of those pairs were compile errors.
`order.Deps.InventoryReserve`, `InventoryDeduct` and `InventoryRestore` took
`inv.Reserve`, `inv.Deduct` and `inv.Restore` — three different slice values
whose method sets did not overlap — so `InventoryDeduct: inv.Reserve` did not
build. Flattening `inventory` made `inv` one value satisfying all three, and
the same thing happened at every flatten. `order`'s was the largest: one
`*order.Service` then satisfied ten port fields across four consumers, and
eight compile errors went with it — three pairs on `payment`, three on
`checkout`, two on `shipping` (its `OrderShip`/`OrderDeliver` pair already
took the same value, so that one was never guarded), none on `review`, which
has a single field. What replaces those eight is an eye: read the value when
you add a port to a consumer that already has one.

**What you would do:** nothing, until a second value shows up on one consumer.
A distinct wrapper type per port would restore the compile error and
reintroduce exactly the pass-through packages decision 4 rejects. A
`bootstrap` test asserting each field holds the value it should is a test of a
struct literal against itself. The mitigation that will matter on the day the
second value arrives is `test/e2e`: a wrong value that changes behaviour on the
paid-checkout path fails the saga, and one on a path e2e does not run does not.

**Decision 17 spent the same fact a different way.** If nothing tells two
fields wired to the same producer apart, splitting that producer's port by
capability was not buying the compile-time safety it looked like it was
buying, so `order`, `payment`, `checkout`, `shipping` and `product` collapsed
their per-capability ports into one interface per producer. That closes the
gap this entry describes — a consumer no longer has several fields for one
producer to mismatch — but it opens a different one: a `ports.go` file used
to double as documentation of the narrowest set of methods a given call path
needed. `shipping.OrderShipper` was one method, and any reader could tell
`MarkShipped` was all `shipping` asked of `order` for that path. `shipping`'s
single `Orders` port now carries `Snapshot`, `MarkShipped` and
`MarkDelivered` together, so the same reader has to open `shipping/service.go`
and follow which method each caller actually invokes — the port itself no
longer proves it. The same is true of `order.Cart` (`Lock`, `Snapshot`,
`Clear` for what `order.Service.Place` alone needs) and every other port that
used to be split by capability. Nothing but reading the `Service` method
bodies recovers the narrowest-possible-dependency view a `ports.go` file used
to give away for free.

## The public and admin response mappers are one wrong import away from each other

**Where you hit it:** you never do, which is the problem.

`user/adapter/http` holds `toUserResponse` (five fields: `id`, `email`,
`first_name`, `last_name`, `phone`) and `toAdminUserResponse` (nine — it adds
`role`, `active`, `created_at`, `updated_at`). Both take `*domain.User`. Both
are in `package http`. `response.OK(w http.ResponseWriter, data any)` accepts
either. So writing `response.OK(w, toAdminUserResponse(u))` inside
`Handler.Me` — the authed `GET /api/users/me` — publishes a user's role and
account state to any authenticated caller, and every gate in the repo lets it
through:

- **it compiles**, because `response.OK` takes `any`;
- **`TestHandler_Me/success` passes**: it unmarshals `email` and `first_name`
  from the body and asserts those two, so extra keys are ignored;
- **the leak test in the same file passes**: it calls `toUserResponse`
  *directly* and asserts its five keys, so it tests the mapper, not the
  handler;
- **`router_test.go`'s `GET /api/users/me` subtest passes**: it asserts `200`
  and nothing about the body;
- **`TestRouteAccess` passes**: it asserts the route 401s an anonymous caller
  and does not 403 a user, which says nothing about the body.

`category/adapter/http` and `product/adapter/http` carry the same pair
(`toCategoryResponse`/`toAdminCategoryResponse`,
`toProductResponse`/`toAdminProductResponse`). Under decision 14 the two
mappers lived in different packages — one per slice — so the wrong one was not
in scope at the call site. The http split narrowed this some: each pair now
lives in two differently-named files, `response.go` and `admin_response.go`,
rather than side by side in one file. Both are still in the same package and
both compile in `response.OK(w, ...)` regardless of which file declared them,
so the risk moved from mistyping an adjacent identifier to reaching for the
wrong file — smaller, since a reviewer scanning the diff sees which file
changed, but not gone, since `admin_response.go`'s mapper is still one import
away from any handler in the package.

This matters here specifically because this codebase controls field exposure
by **DTO omission** rather than by `json:"-"` (decision 9). The point of that
inversion is that publishing a field has to be a deliberate act. Naming the
wrong mapper is a deliberate-looking act that publishes four extra fields.

**What you would do:** for each public route that has an admin twin, assert
the exact key set of the response body through the handler rather than through
the mapper. Six routes need it: `GET` and `PUT /api/users/me`, `GET
/api/categories` and `/api/categories/{slug}`, `GET /api/products` and
`/api/products/{slug}`. Not done here — the decision belongs with
whoever reviews the whole shape, because fixing three modules and not the
other twelve manufactures a consistency that is not there.

## Nothing tests the middleware chain `NewRouter` builds, or either rate limiter's binding

**Where you hit it:** you reorder `web.Chain`'s arguments, or bind a
route group to the wrong limiter, and the whole suite stays green.

Both pieces test each middleware in isolation —
`internal/platform/web/middleware`'s `recovery_test.go`, `requestid_test.go`,
`logging_test.go`, `cors_test.go`, `role_test.go` and `ratelimit_test.go`,
and `internal/server`'s `auth_test.go` — all exist. Nothing tests the
composition, and the split still costs the same thing: the chain
`NewRouter` builds spans two packages, `internal/platform/web` (`Chain`
itself) and `internal/platform/web/middleware` (every argument passed to
it), so neither package's tests can cover it even in principle.
`internal/server/router.go`'s `NewRouter` ends with:

```go
return web.Chain(
	middleware.RequestID,
	middleware.Logging(logger),
	middleware.Recovery(logger),
	middleware.CORS(appCfg.CORS),
)(mux)
```

`internal/server/router_test.go` asserts CORS headers, but has no
request-ID-correlation test and no panic test. Two things therefore go
unproven:

- **`Recovery` is third.** A panic raised inside `RequestID` or `Logging`
  escapes unrecovered. This is not a defect the flatten introduced — the order
  is identical at every commit on this branch and before it — and it is not
  fixed here, but it is exactly the class of bug a chain-level test catches and
  nothing in this repo would.
- **Neither rate limiter's binding is visible to any test.** `NewRouter`
  builds two, and both are group middleware: `authLimiter` on `authPublic`,
  and `orderLimiter` on `orderWrites`, a group derived from `authed`
  with an empty prefix so it adds a limiter without adding a path segment
  (`POST /api/orders`, `POST /api/orders/{id}/pay`).
  `TestRouteAccess` classifies a route by whether an anonymous request gets
  401 and an authed one gets 403 — which a limiter does not change. Detach
  either limiter from what it guards and not one assertion fails. Deleting
  the golden did not cost this: the golden's group column was chosen to
  match what the probe observes, so it never saw a limiter either.

The near-miss is worth recording, because it was real. Before the fourteen
route files collapsed into one, `routes.Auth`'s first parameter was named
`api` while `router.go` called it as `routes.Auth(authPublic, ...)`. Inlining
those files by parameter name — the obvious mechanical way to do it — would
have mounted all three auth routes on the unlimited `api` group, dropping
login rate limiting entirely, with `make all` green and the golden unchanged.

**What you would do:** two small tests, neither expensive. For the chain,
register a handler that panics and assert a 500 with a request-ID header, and
assert the header round-trips on a normal request. For the limiters, drive
`POST /api/auth/login` past `AUTH_RATE_LIMIT` and assert a 429, and the same
for `POST /api/orders` against `ORDER_RATE_LIMIT`. Both can use the
Redis-backed router test that already exists. Neither is done here.

## A sentinel's HTTP status is fixed where the sentinel is declared

**Where you hit it:** you want to know why `POST /api/orders/{id}/cancel`
answers 409, and you open `internal/platform/web/response/error_mapper.go`
to find out. It does not say.

`HandleErr` is five rows long. It matches `errs.ErrNotFound`,
`errs.ErrConflict`, `errs.ErrBadRequest`, `errs.ErrUnauthorized` and
`errs.ErrForbidden`, in that order, and returns 500 for anything else. The
status for `apperror.ErrOrderCharging` is decided one line earlier than you
would look, in `internal/apperror/apperror.go`:

```go
ErrOrderCharging = fmt.Errorf("%w: order has an in-flight payment, cannot cancel", errs.ErrConflict)
```

That inversion is the price of the thing it bought, and the trade is a good
one: the mapper carries no business knowledge at all, which is what let it
move below `internal/modules/` into `internal/platform` and stop being a file
every new sentinel had to be registered in. The old fifteen-row table was a
second place to remember, and `ErrAlreadyFinalized` is the proof it did not
work — it was missing from that table and returned 500 on a live route until
this split gave it a wrap.

What you give up is the single readable index. There is no file that lists
"these errors are 409", and answering the question means grepping for the
sentinel and reading its declaration. There is also no compiler help for the
mistake that replaced the old one: a sentinel declared with a bare
`errors.New` unwraps to none of the five kinds, so it 500s, and
`internal/apperror/apperror_test.go`'s hand-written table does not grow a row
on its own. `AGENTS.md`'s code-style section states the rule; nothing enforces
it.

**What you would do:** an `apperror` test that reflects over the package's
exported `error` variables and asserts each one matches one of the five kinds
would close the gap without a table to maintain. It needs `reflect` over a
package's own top-level vars, which Go does not offer directly — the practical
form is a small `go:generate`d list or a `go/ast` walk in the test. Neither is
done here, and with seven sentinels the table is still cheaper than the
machinery.

## The copy property `internal/platform` is checked for holds for `go build`, not `go test`

**Where you hit it:** you `cp -r internal/platform` into a fresh module, as
check 8 exists to guarantee you can, and `go build ./...` passes and `go test
./...` does not compile.

Check 8 forbids every import of local code from under `internal/platform`
except `internal/platform` itself — and `internal/testutil`, its one
exemption. That exemption is not decorative: `platform/database`,
`platform/cache` and `platform/queue` each import `internal/testutil`
from a `_test.go` file, for the shared dockertest harness that starts the two
fixed-name containers. `internal/testutil` does not live under
`internal/platform`, so it does not travel with a copy of it, and those three
test packages fail to build in the fresh module.

The non-test build is genuinely a leaf, which is what the goal was worth
stating. The test build never was, and was not made worse by the split — the
three imports predate it. The reason it is written down rather than quietly
tolerated is that check 8's exemption list would otherwise read as a closed
set with an unexplained hole in it, and the next person to widen the check
would remove the exemption, break three test packages and learn this the
expensive way.

**What you would do:** move `internal/testutil` to
`internal/platform/testutil` and drop the exemption. It is a mechanical move —
one directory, 25 calling test packages, an import-path rewrite — and it makes
the leaf property true for `go test` as well. The argument against is that
`internal/testutil` is a test harness rather than infrastructure the running
binaries use, and putting it under `internal/platform` says otherwise. Not
done here.

## `cmd/` and `test/` are outside every boundary check

**Where you hit it:** you look for the check that stops a binary reaching into
a module's `domain/`, and there is not one.

All six checks walk `internal/` and nothing else. Check 1 finds its files
with `find internal -type f -name '*.go'`; check 4's `importer_roots` iterates
`internal/*/`; check 6 iterates `internal/modules/*`; check 8 walks
`internal/platform`. Check 8 is the only one that so much as names `cmd/`, and
only as a *target* — it reports a platform file importing
`cmd/mockgateway/mockserver`, never anything `cmd/` itself does. So `cmd/` and
`test/` are never scanned as importers, and both use the freedom: `cmd/worker/main.go` imports
`payment/domain` for the `Job` type its processor's methods take, `test/e2e`
imports it to assert on, and `cmd/mockgateway/mockserver` imports
`payment/gateway` and declares three `json` tags of its own. None of it is
reported.

This is not new — the same imports were there at the last commit before the
flatten, plus `payment/worker`, since deleted — and none of them is obviously
wrong: a binary is wiring, and `test/e2e` exists precisely to reach across
modules. What is missing is the *statement* of that in a form a check could
hold. `internal/bootstrap` and `internal/server` are exempt through an
explicit, argued `WIRING_DIRS` entry. `cmd/` and `test/` are exempt through
never having been in scope, which is the same kind of silence check 4's old
denylist used to keep.

**What you would do:** add `cmd/` and `test/` to check 4's walk *without*
adding them to `WIRING_DIRS`, then decide each surviving import on its merits
— `payment/domain` in `cmd/worker` looks like a published type that belongs in
`payment/contract.go`, and `payment/gateway` in the mock gateway is genuinely
someone else's contract. That turns three unexamined imports into three named
ones. Half an hour, and not done here.

---

## You cannot filter or sort a product listing by stock

**Where you hit it:** you try add `?in_stock=true` to `GET /api/products`.

`products` owned by `product`, `inventory_levels` owned by `inventory` (decision 6, decision 7), so listing query cannot join them. Port is `product.Inventory.GetAvailability(ctx, ids)` (`internal/modules/product/ports.go`) — batch-shaped, asked _after_ page already selected. `product.Service.ListPublished` calls `repo.ListPublished` then `enrich`, that order, cannot be other order: enrich need ids page chose.

So filter apply only to rows already fetched, and that break pagination, not merely slow it. `ListPublished` keyset-paginated on `(created_at, id)`, fetch `Limit + 1` rows to decide `hasMore`. Ask 20, drop 8 out of stock, get page of 12 whose cursor claim client stopped at row 20. Repeat: page sizes wobble unpredictably while `hasMore` lies. Sort by stock worse: sort key not in table `ORDER BY` run against, so nothing to sort on until window chosen.

Today's filters — `category_id`, `min_price`, `max_price`, `search` — work because they columns of `products`.

**There is no partial fix.** Fetching extra rows to compensate not one: cannot know how many extra, and cursor still must name row you did not return.

**What you would do:** build read model `ARCHITECTURE.md` reject for now.

```text
product_view
  product_id, name, slug, price, currency, status, category_id, available_stock
```

`inventory` write it on every level change — same transaction if want exact, async if accept lag — and `GET /api/products` read one table it can filter, sort, paginate freely. That new table, new writer, decision about staleness. It a feature, and right one day storefront need it. Left out because read model with no consumer is speculative infrastructure.

## Multi-warehouse is a redesign, not a column

**Where you hit it:** second warehouse appear, you reach for `ALTER TABLE inventory_levels ADD COLUMN warehouse_id`.

`inventory_levels` is `PRIMARY KEY (product_id)`. Ratified, not overlooked — migration say so in comment, `ARCHITECTURE.md` list multi-warehouse under Rejected. Single-row-per-product invariant load bearing in four places; adding column break all four silently:

1. `Reserve` update `WHERE i.product_id = v.product_id AND
i.available_stock >= v.qty`. With several rows per product, one 1-unit reserve increment `reserved_stock` in _every_ warehouse row.
2. `if int(tag.RowsAffected()) != len(ids)` is insufficient-stock signal. Once rows and products not same count, that comparison stop meaning anything — and fail open or closed depending how many warehouses happen to stock item.
3. Availability stop being row predicate. `available_stock >= qty` become `SUM(available_stock) >= qty GROUP BY product_id`, which no single guarded `UPDATE` can enforce without first deciding which warehouse each unit come from.
4. Deadlock avoidance rest on `SELECT 1 FROM inventory_levels WHERE product_id
= ANY($1) ORDER BY product_id FOR UPDATE`. Composite key need composite lock order, and argument why concurrent checkouts cannot deadlock stop being obvious enough to trust.

Outside `inventory` also need allocation policy (fill-first? nearest? split line?), `warehouse_id` on `order_items` so release or deduction target row units came from, per-line reservation _records_ not counters so refund can return units where they came from, and split-shipment support in `shipping`.

**What you would do:** plan it as feature touching four modules and adding a table. What you must not do is add nullable `warehouse_id` every query ignore — worse than absence, because next reader will assume it works.

## Two queries where one join would do

**Where you hit it:** you read `product.Service.ListPublished` and count round trips.

Every product listing and every single-product read cost second query to `inventory` for same ids. Deliberate price of decision 6.

Trade is bounded — only reason acceptable: one extra query per _page_, not per row, and not grow with page size. Port batch-shaped (`GetAvailability(ctx, ids)`) specifically so N+1 version awkward to write.

**If you ever see one inventory call per product, that is a bug, not the design.** Look like `for` loop around single-id lookup.

**What you would do:** nothing, until profiling say otherwise; then read model above collapse it back to one query.

## Creating a sellable product takes two admin calls

**Where you hit it:** you `POST /api/admin/products` and product have no stock.

`product` may not write `inventory_levels`, so create path can only ask `inventory` to materialise row via `product.Inventory.EnsureLevel`. Setting actual quantity is separate call to `inventory`. No `stock_quantity` field on product's write DTOs — removed, and absence is the point.

Alternative is `product` writing inventory's table inside own transaction — precisely the violation decision 6 exist to remove.

**What you would do:** accept it, make client do both calls. If one-call creation matter, that orchestration concern, belong in caller holding both ports — not in `product`.

## The cart is not a quote

**Where you hit it:** price change between customer adding item and checking out, cart total change under them.

`cart_items` have columns `id, cart_id, product_id, quantity, created_at,
updated_at`. **No price column**. `cart.Service.Get` read lines then call `products.GetInfoByIDs` for current name, price, status, available stock. So cart not display stale price — never pinned one at all. Every read current.

Note this cut against intuitive framing. Cart's total never _stale_; it **unpinned**. What genuinely point-in-time snapshot is `available_stock` each line report: true when cart read, can be wrong by time `PlaceOrder` run — why checkout re-reserve against `inventory` instead of trusting what cart displayed.

`order` do snapshot prices at placement, so an _order_ internally consistent forever. Cart not, and not meant to be.

**What you would do:** if need held price, add `unit_price` and `currency` to `cart_items`, write them at add-time, then decide the thing that actually make this hard — what happen when held price and current price disagree at checkout. Reprice silently? Refuse? Show both? Price column without answer to that question just move surprise later.

## An unsellable cart line is shown, not hidden

**Where you hit it:** product archived, unpublished or deleted after customer added it, and `GET /api/cart` still return line — with `"sellable": false`, excluded from `total`.

Behaviour change, and chosen. Previous implementation drop line with `JOIN … AND p.deleted_at IS NULL`, so customer's total fell with nothing on screen to explain. If product record gone entirely, `cart.Service.Get` substitute synthetic `&Product{Status: "unavailable"}` placeholder instead of dropping item.

**What it costs:** every client rendering cart must handle `sellable: false`, and client written against old behaviour will show line it cannot check out. `Cart.Total()` fold sellable lines only, so `total` will not equal sum of line prices client can see — look like bug if you not read this.

**What you would do:** render unsellable lines distinctly, offer remove action. Do not re-add the `JOIN` filter.

## A mixed-currency cart is a 400 from `GET /api/cart`

**Where you hit it:** cart contain items priced in different currencies, endpoint that used to return 200 now return 400.

`money.Money.Add` refuse to sum across currencies, so `cart.Cart.Total()` return `(money.Money, error)`, and `internal/modules/cart/adapter/http/handler.go`'s `Get` propagate that error instead of publishing total it could not compute. Error wrap `errs.ErrBadRequest` alongside `money.ErrCurrencyMismatch`, because `ErrCurrencyMismatch` alone match no case in `response.HandleErr` and would surface as 500 for what is plainly user input.

**Nothing prevents such a cart existing.** Prices per-product and `cart.AddItem` not constrain them, so catalogue with mixed currencies will produce this. Checkout already reject it; this change make `GET /api/cart` agree with `PlaceOrder` instead of showing number denominated in nothing.

Two things soften it, both deliberate: `Total()` fold **sellable lines only**, so archived line in foreign currency still yield clean 200; and empty cart yield `total: 0` not error.

**What you would do:** decide currency at level above product. Constrain catalogue to one currency, or scope carts by currency and reject the add not the read. The 400 is correct report of inconsistent cart; not the place to fix it.

## `promotion` and `dashboard` amounts are plain `int64`

**Where you hit it:** you reach for `money.Money` in `promotion` or `dashboard` and find nothing to construct it from. Neither package reference `money` at all.

`money.Money` cover exactly four features — `order`, `payment`, `product`, `cart`. `ARCHITECTURE.md` §10 state why other two are out, and reasons load-bearing not bookkeeping:

- `Promotion.Value int64` is **polymorphic**. With `Type == TypePercentage` it a percentage; with `TypeFixedAmount` it minor units. `money.New(10,
"USD")` to mean "10%" would be value object asserting something false. `promotion` also have no currency field anywhere, so its genuinely monetary `MinOrderAmount`, `MaxDiscount` and `CouponUsage.Discount` have nothing to pair with.
- `dashboard` aggregate revenue across all orders and have no currency field, so any currency it named would be guess.

**What it costs:** seam between `order` and `promotion` untyped in middle. `order.CouponReserver.Reserve` (`internal/modules/order/ports.go`) pass `orderSubtotal int64` and return `discountAmount int64`, and `order.Service.Place` re-pair it with `money.New(discount, subtotal.Currency)` on own side. That also where clamp live — `max(subtotal-discount, 0)` as plain arithmetic, because `Money.Sub` deliberately not decide whether money may go negative. If you add multi-currency promotion system, this seam where type safety missing and where wrong-currency discount would pass unnoticed.

**What you would do:** give `promotion` a currency column and split `Value` into two fields — a percentage and a `Money` — before trying to use `Money` across that port. Retrofitting `Money` onto polymorphic column first will produce type that lies.

## Extracting a module into a service is a data migration, not a refactor

**Where you hit it:** you decide to pull `order` out into own service and discover Go part is easy half.

Code boundaries genuinely clean — no module import another, every cross-module call go through interface the _consumer_ declared, so each module's port list already the API its service would expose. Database is one schema with **22 foreign keys, 16 of which cross a module boundary** (measured against `pg_constraint` on migrated database; full list and the 6 internal ones in `db/OWNERSHIP.md`).

Those 16 exactly what make split a data problem. Cannot put `orders` and `products` in separate databases while `order_items.product_id` carry foreign key. Step one of any extraction is dropping 16 constraints and re-expressing each as application-level check _with explicit answer for the race the constraint used to close_ — because port check at different moment than the one the write commits in, and constraint have no such window. That a migration with correctness argument attached, not a refactor.

One of the 16 load-bearing in Go not merely defensive. `products.category_id` is only cross-module constraint that can fire in normal operation, because `categories` is only hard-deleted table another module reference; `category`'s adapter catch violation as backstop behind `category.ProductCounter`'s friendlier pre-check. Drop that constraint and backstop go with it.

**What you would do:** budget data split as own project. Every port declared in this refactor make code side cheap; none of them touch this side.

## Foreign-key fan-in is not the dependency graph

**Where you hit it:** you try work out which module to extract first by looking at which tables everything reference, and get answer nearly inverted.

| Module      | Inbound FKs | Inbound ports |
| ----------- | ----------- | ------------- |
| `user`      | 6           | **1**         |
| `order`     | 5           | **4**         |
| `product`   | 6           | **2**         |
| `inventory` | **0**       | **3**         |
| `category`  | 2           | 0             |

Inbound FKs count constraints referencing table the module owns. Inbound ports count interfaces _other_ modules declare that this module's service satisfies — `auth.UserDirectory`, `payment.Orders`, `product.Inventory` and so on. Derive the port column with `grep -n 'type .* interface' internal/modules/*/ports.go` and attribute each name to the producer it asks for: `order`'s four come from four modules (`payment` 1, `checkout` 1, `shipping` 1, `review` 1), `inventory`'s three from three (`order` 1, `payment` 1, `product` 1). Both FK counts read one lower than they used to: the retired `notification_jobs.user_id` referenced `users`, and the retired `payment_jobs.order_id` referenced `orders`, so `user`'s count dropped from 7 to 6 and `order`'s from 6 to 5 when those two tables went — decision 18 in `ARCHITECTURE.md`.

`users` most-referenced table in schema and almost nothing call into `user`: six tables carry `user_id`, and caller writing one already **has** the id, so nothing to ask. `inventory_levels` have no inbound foreign keys whatsoever and three interfaces across three modules declare ports against `inventory`, because stock is answer that _changes_ and must be asked every time.

**Foreign-key fan-in measures how many tables carry an identity. Port fan-in
measures how much behaviour other modules need.** Close to independent, and neither alone tell you what coupling costs.

`orders` only table high on both. That — not its FK count — the real argument `order` is hardest module to extract and the one to modify most carefully.

**What you would do:** when planning extraction, count ports first, constraints second. Ports tell how much runtime coupling you must replace with network calls; constraints tell how much of data migration you must argue for.

## `make check-boundaries` has blind spots, and they are where you would hide

**Where you hit it:** you assume green `Boundaries OK` mean boundaries hold. It mean boundaries hold _in the places the script can see_.

Five checks, all greps, none a compiler. `db/OWNERSHIP.md` documents check 3's (table ownership) gaps in full; the ones most likely to bite:

- **Table names must be literals.** Check 3 greps for the identifier after `FROM` / `JOIN` / `INSERT INTO` / `UPDATE` / `TRUNCATE` / `COPY`. Every query today has its table name in a string literal, but `fmt.Sprintf` is already routine in these adapters for `WHERE` fragments and placeholder lists. The habit of assembling SQL exists; it simply has not reached a table name yet. The day it does, the check goes quiet instead of failing. `pgx.CopyFrom` would be the same hole immediately: its table is a `pgx.Identifier` Go value with no keyword in front. Nothing uses it today.
- **Prose in a production string literal is a false positive.** Comments and `_test.go` files excluded, but `"update orders failed"` in a module's `adapter/postgres` — or in its `service.go`, now that the whole module is scanned — still reports `orders`. Fails loudly, not quietly, which is the right direction, but it is the failure mode that gets a check disabled.
- **Test files are skipped, deliberately.** A test seeds sibling tables to satisfy foreign keys — fixture setup, not an architectural crossing. Cost: a real violation living in a `_test.go` helper is invisible.
- **`dashboard` is exempt wholesale.** Nothing verifies it stays read-only — no grant, no separate role, no check. An `UPDATE` in `internal/modules/dashboard/adapter/postgres` passes CI today. Nothing bounds it to today's two tables either, so "may read anything" can become a second, undeclared copy of the schema without a review comment.
- **Ownership is per table, so column coupling is invisible.** `dashboard` depends on `order_items.unit_price`. Rename that column and `dashboard` breaks at runtime, in a query only an admin runs, with `go build` unable to see inside the SQL string.
- **Nothing the database does on its own.** A view, trigger or function crossing modules is not Go source, not scanned.

Four blind spots sit outside check 3 entirely and each gets its own entry
rather than a bullet here, because each has its own "where you hit it": [a
module's exported surface](#a-modules-whole-exported-surface-is-reachable-from-every-other-module)
and [`checkout`'s exemption](#checkout-is-held-to-a-weaker-boundary-rule-than-its-fifteen-siblings)
above, [`cmd/` and `test/` being unscanned](#cmd-and-test-are-outside-every-boundary-check)
above, and [SQL built from a string](#a-module-can-still-smuggle-sql-past-check-3-with-a-built-string)
below. The shape they share: every check matches a **quoted import path** or a
**string literal**, so what a check can see is a spelling, never a call.

**What you would do:** treat the checks as a ratchet against regression, not proof of correctness. If you start building table names dynamically, replace the grep with a parser or ban the pattern — do not widen the allowlist.

## A path-keyed check can quietly stop matching anything

**Where you hit it:** you bisect a boundary violation and find it was
introduced in a commit where `make check-boundaries` passed.

This has happened, twice, and it is the failure mode of every check in this
script. Checks 1 and 6 key off a path. When the modules were sliced, check 1
exempted `internal/modules/<feature>/usecase/<slice>/http/` and check 3 scanned
only a directory named `postgres`; from the moment a module was sliced until
the checks were rewritten, every path either check keyed off had already moved
deeper than the check expected, so both matched nothing in that module and
reported nothing. It was verified at the time by planting a `json` tag on a
domain type and a foreign `FROM orders` in a use case, both of which passed a
green `check-boundaries`. The flatten set the same trap in the other direction:
those `usecase/` exemptions matched nothing at all once the slices were gone,
and an exemption that matches nothing is indistinguishable from a deleted one
until the linter starts complaining about the wrong thing.

The answer is `scripts/boundaries_test.go`, and it now probes from both sides.
It plants a probe file in a real module and asserts each check reports it — a
`json` tag, a `json:"-"`, a `dto.go`, a foreign `FROM orders`, a `domain/`
import, an `adapter/postgres` import, an `internal/server` import. It also
plants files that must **not** be reported: a json tag inside `adapter/http`,
a root-package import between two modules, an adapter import from
`internal/bootstrap`, `checkout` importing `order/domain`. Those are the
subtests that catch a dead exemption, because a check that has stopped
matching anything passes the negative probes for the wrong reason and fails
the positive ones.

Every surviving exemption also has a number behind it, established by removing
it and counting rather than by reading its comment: check 1 without its
`adapter/http` arm reports 295 violations, check 6 without its `adapter/http`
arm reports 85 across fifteen modules, and `WIRING_DIRS` emptied reports 30.
Three exemptions that produced *zero* when removed were deleted for exactly
that reason.

**What you would do:** on the next structural migration, rewrite the checks
against a hand-built pilot module first and run both shapes — old check, new
tree — in parallel, rather than rewriting the checks at the end. Both times so
far, the first green run of the real checks came after every module had
already moved, which is backwards from when a boundary check is most useful.
The probe test is the ratchet that stops it being paid a third time; it is not
a substitute for doing the checks first. And when you remove an exemption,
count what appears. "The regex looks right" is how this branch shipped three
dead exemptions.

## A module can still smuggle SQL past check 3 with a built string

**Where you hit it:** you write `db.Query(ctx, "SELECT * FROM "+table)` or hand a variable to `pgx.CopyFrom`, and `make check-boundaries` says nothing.

Check 3 scans every non-test `.go` file under a module, so SQL in `service.go` is caught exactly like SQL in `adapter/postgres` — the directory-scoped hole the previous entry describes is closed. It still only matches string literals. A table name assembled with `+` or `fmt.Sprintf`, or reaching `pgx.CopyFrom` as a `pgx.Identifier` value, is invisible either way; widening which directories get scanned changes nothing about what the pattern itself can see.

**What you would do:** nothing cheap. A real fix parses Go. The literal-only scan catches every violation anyone has written by accident, and none written on purpose — which is the trade every grep-based check in this script makes, not a gap unique to this one.

## The read replica is wired up, with no protection against reading your own write

**Where you hit it:** you create a promotion through `POST /api/admin/promotions`, immediately `GET /api/admin/promotions`, and the row is missing — or you set `REPLICA_DATABASE_URL` for the first time and a report changes by a few seconds' worth of orders.

This used to be a dead seam — configured and unused. It is not any more. `database.NewReplicaPostgres` builds a second pool from `REPLICA_DATABASE_URL`; `internal/server/server.go` and `cmd/worker/main.go` both construct it at boot (falling back to `nil`, logged as a warning, if it fails to connect or the variable is unset) and pass `database.DB{Primary: pool, Replica: replicaPool}` — one value — through `bootstrap.New` to every module's postgres adapter. `database.PrimaryDB(ctx, db)` and `database.ReplicaDB(ctx, db)` both check the context for an open transaction first; `ReplicaDB` falls back to `Primary` when no replica is configured, so the seam degrades to today's single-pool behaviour by default rather than failing.

**Five read-only methods across five adapters call `ReplicaDB`:** `order.ListAdmin`, `product.ListAdmin`, `promotion.ListAdmin`, `user.ListAdmin`, and all four of `dashboard`'s (`GetSalesSummary`, `ListOrderStatusBreakdown`, `ListRevenueByDay`, `ListTopProducts`). Every other read and every write goes through `PrimaryDB`.

**Nothing marks a request as having just written, so nothing routes a read-your-own-write back to the primary.** The mechanism this limitation used to say was missing — a `WithRecentWrite` context marker, applied by middleware after a non-`GET` — was never built, and nothing replaced it. `order.ListAdmin` and the other four `ListAdmin` methods can read a replica that has not yet caught up with a write the same admin just made, and there is no code path that would notice or compensate. Today's blast radius is small — admin list views, not the checkout path, and replication lag is normally milliseconds — but it is a real, live gap now, not a hypothetical one behind an unused flag: setting `REPLICA_DATABASE_URL` for the first time is the day this starts being observable.

**What you would do:** either accept the gap and document the lag it can show (an admin who just created something and lists it back may briefly not see it — say so at the two call sites most likely to be driven by a human clicking immediately after a write, `promotion` and `product`), or add the `WithRecentWrite` marker this entry used to describe as missing and thread it through the four admin-write routes ahead of their sibling list routes.

## RESOLVED: the charge job was dispatched but never enqueued

**What it was.** `payment/adapter/jobs.Dispatcher.Process` switched on `job.Action` with a `case domain.ActionCharge: return d.charge.RunChargeJob(ctx, job)`, and reasonably read as though charges ran through the job queue the way refunds did. No production code ever created a `payment_jobs` row with `action='charge'` — every call site that created a job went through `Service.enqueueRefund`, which hardcoded `domain.ActionRefund` — so `Service.RunChargeJob` was unreachable outside tests even though the dispatcher routed to it correctly. Charging happened inline instead, and `FinalizeSuccess` ran a `MarkJobCompleted` against a synthetic, zero-`ID` `Job` for both of its inline callers, silently matching zero rows every time.

**Why it is gone.** The whole mechanism this entry described no longer exists. `payment/adapter/jobs.Dispatcher`, `domain.ActionCharge`, `Service.RunChargeJob` and `payment_jobs` are all deleted — decision 18 in `ARCHITECTURE.md`. Payment now declares exactly one job type, `RefundArgs` (`payment/adapter/jobs/refund.go`), registered as a `river.Worker` in `internal/worker`; there is no charge job to dispatch and nothing pretending there is. Charging is still inline, on the same two paths (`Service.Charge` when the gateway captures synchronously, `Service.HandleWebhook` when it does not) — that part of the design was never wrong, only the appearance of a second path through the queue that no caller took. There is no longer a `RunChargeJob` for anyone to read `Dispatcher.Process` and mistake for one.

## Job delivery is at-least-once, and only one of the two jobs is idempotent

**Where you hit it:** a job's handler succeeds, but the process dies or loses its database connection before the follow-up commit lands. The job stays `running` until `WORKER_RESCUE_AFTER` lapses, and River's own rescuer reclaims and re-runs it — by design, not by accident. River promises at-least-once delivery, not exactly-once, and hands the difference to whoever writes `Work`.

**This is a design decision, not an oversight**, and the two jobs that matter here sit on opposite sides of it:

- **`payment.RefundWorker` is idempotent twice over.** `SettleRefund` refuses any payment whose status is neither `success` nor `requires_review` before it calls the gateway — returning `payment.ErrNotRefundable`, which `RefundWorker.Work` translates into `river.JobCancel` so a re-run against an already-refunded payment is cancelled outright rather than retried or refunded twice. The gateway call itself also carries `IdempotencyKey: p.ID.String()`, which is what actually stops a genuinely concurrent double-execution — two workers racing the same reclaimed job before either has updated the row — from refunding twice at the gateway; the guard alone cannot see that race, since both would read `success` before either writes. Underneath both, the status write to `refunded` is a compare-and-set naming its allowed predecessors (`success`, `requires_review`), so at most one of the two concurrent runs can also win the database update.
- **`notification.SendWorker` is not idempotent**, and is accepted as at-least-once anyway, because the only channel this template ships, `notification/adapter/channel/log`, turns a duplicate send into a duplicate log line. Nothing downstream of `Channel.Send` de-duplicates.

**What a real channel implementation owes.** Swapping `adapter/channel/log` for an email or SMS sender inherits the at-least-once guarantee whether it wants to or not — River does not change, only the channel does. A production channel needs one of two things before it ships: idempotent delivery at the provider (many transactional-email APIs accept a client-supplied message ID for exactly this), or a delivered marker `SendWorker.Work` checks before calling `Channel.Send` and sets after. Shipping neither means the first rescued retry, database blip, or `cmd/worker` restart mid-job sends the same email or SMS twice. Learn this here, not from a support ticket about duplicate mail.

**`checkout.Service.CancelPendingByOrderID`'s failure is logged, not propagated, for the same reason.** It now does two things where the old system did one: `payment.Service.CancelPendingByOrderID` calls `payment/adapter/jobs.Queue.CancelPendingForOrder`, which lists every pending, scheduled or retryable refund job tagged with the order and cancels each via `river.Client.JobCancel` — an active step the old system never had. But if that call itself fails and the error is swallowed, the fallback is the same guard `RefundWorker` leans on: a refund job that eventually runs against a payment already moved past `success`/`requires_review` simply no-ops. The worst a swallowed cancellation error does is leave that no-op job sitting in the queue, not double-refund or lose a cancellation that mattered.

## What the shared job table gives up

**Where you hit it:** two separate ways, and both survived the move from a hand-rolled `job_queue` to River's own `river_job` — a shared job table gives these up regardless of who built it.

- **No foreign key ties a job row to the thing it is about.** `payment_jobs.payment_id` referenced `payments(id)` and `notification_jobs.user_id` cascaded from `users(id)`; `river_job` has neither, verified against `\d river_job` on a migrated test database — no `Foreign-key constraints` section at all. A table naming `payments` or `users` in a constraint would be exactly the cross-module reach decision 6 forbids a module's own SQL from making, and `river_job` is not this repository's table to add one to even if that reach were allowed. `users` and `notifications` are both soft-deleted today, so nothing currently exercises the gap — but a deleted user's pending `notification.send` jobs are not cleaned up by any constraint the way `notification_jobs.user_id`'s old cascade would have done it. Whatever eventually removes a `notifications` row — a hard-delete path, a retention job, GDPR erasure — leaves any job still referencing it to fail `SendWorker.Work`'s notification lookup on a missing row, on every attempt, until River discards it after `MaxAttempts`.
- **A job's args are JSON matched by field name, and nothing checks it against the struct that will decode it.** `river_job.args` is `jsonb`; River marshals a `river.JobArgs` value into it with `encoding/json` on enqueue and unmarshals it back into a zero value of the registered `Kind()`'s args type when a worker claims it. Renaming an exported field on `payment/adapter/jobs.RefundArgs` or `notification/adapter/jobs.SendArgs` is silent for every row already sitting in `river_job` at the moment of deploy — the old field name in the stored JSON simply has nowhere to land, and the Go field keeps its zero value with no error from `json.Unmarshal`. No compiler sees this, because the write side and the read side are decades of wall-clock time apart, not two call sites in one build.

**One old cost is gone rather than merely renamed.** `jobs.ErrDiscard` was a bare sentinel, and `errors.Is` decided whether an error meant "discard this job, do not retry it" — a handler returning an error from a library that happened to wrap `jobs.ErrDiscard` internally would have been silently treated the same way, with nothing to tell the two apart. River's equivalent, `river.JobCancel(err)`, returns `*rivertype.JobCancelError`, a distinct type River's own executor detects with `errors.As`, not `errors.Is` (`internal/jobexecutor/job_executor.go` in the `river` module) — an unrelated library's error would have to construct that exact concrete type to be mistaken for one, not merely wrap a shared value. `payment.RefundWorker.Work` is the one caller today, translating `payment.ErrNotRefundable` into it.

## The test suite shares one Postgres and one Redis, and slots are hand-assigned

**Where you hit it:** you add test package, copy a `TestMain` from neighbour, and another package's tests start failing intermittently.

`internal/testutil` start two long-lived containers by fixed name — `go-api-test-postgres` and `go-api-test-redis` — and every test binary attach to whichever already exist. Isolation by **claimed slot**, not by container:

- **Postgres: one database per module.** `MustStartPostgres(dbName)` create and migrate `dbName` once, under an advisory lock, and nothing ever drop it — `"test_cart"`, `"test_order"`, `"test_payment"`, and so on (`grep -rn 'MustStartPostgres(' --include='*_test.go' internal/modules` is the live mapping). Every module owns its database name outright now: `notification` and `payment` used to have a second `jobs/postgres` test package sharing their name, but that package is gone along with the per-module job queue, and the [two-test-packages entry below](#resolved-two-test-packages-on-one-database-never-got-a-clean-table) is the record of it.
- **Redis: a hand-assigned integer, and no comment tracks it any more.** `MustStartRedis(dbIndex)` takes an index the caller picks by hand; the list below and `grep -rn 'MustStartRedis(' --include='*_test.go' .` are the only record — a comment above the function in `internal/testutil/testutil.go` used to carry this too, but the comment strip removed it, so it is not there to check against even by accident. Indices 0, 1, 3, 5 and 6 are claimed (`platform/cache`, `platform/web/middleware`, `server`, `test/e2e`, `modules/user/adapter/redis`); 2 and 4 are free. `ResetRedis` calls `FlushDB`, so reusing an index flushes another package's fixtures.

Nothing enforce either claim, and losing the comment removed the one place a reader would have looked before guessing. Duplicate name or index compile, pass review, and fail as flake in unrelated package — worst possible signal, because failure nowhere near the change.

Two further consequences worth knowing before writing tests:

- **`t.Parallel()` does not buy anything within a package**, because subtests share one database. Parallelism come from `go test` running package binaries concurrently — exactly why integration tests stay colocated instead of collapsing into one `test/integration` package (decision 11).
- **`make test` cannot run without Docker.** No build tags, no short mode. Every package touching Postgres or Redis fail outright.

**What you would do:** when adding test package, pass the owning module's existing database name, and (if it need Redis) grep the five call sites above for a free index before taking one. If suite grow much past 15 Redis-using packages, index space run out and allocation must become dynamic.

## RESOLVED: two test packages on one database never got a clean table

**What it was.** `notification` and `payment` each had a second `jobs/postgres` test package sharing `test_notification` / `test_payment` with their own `adapter/postgres` package. `MustStartPostgres` creates and migrates a database once, under an advisory lock, and never drops it — dropping it mid-run would tear down whichever sibling package still held it open — so there was no `ResetDB` between those two packages and no clean table to assume, even though `go test ./...` ran them concurrently against the one database.

The flatten had already shrunk this from a general problem to a two-module one: before it, `order` spread `test_order` across **nine** test packages and `payment` spread `test_payment` across **five** (`git grep -l 'MustStartPostgres("test_order")' 0ee2cc5 -- internal/modules`), and 75 packages in the repo called `testhelper.MustStart*`.

**Why it is gone.** `payment/jobs/postgres` and `notification/jobs/postgres` no longer exist — the job queue they backed moved to `internal/platform/jobs/postgres`, one shared package with its own database, `test_platform_jobs` — so neither module has a second test package to share a name with any more. That platform-owned package is gone in its turn now: jobs run on River, and `internal/platform/queue` (`test_platform_jobs_insert`) and `internal/worker` (`test_worker`) are the two Postgres-claiming packages that replaced it, each with its own database and no sibling to share it with. **25 packages call `testutil.MustStart*` today, 22 of them Postgres-claiming, 13 of those inside `internal/modules` — one package per module, every one of the 13 owning its database outright.** No module needs the caution this entry used to state: seed the rows your subtest owns and scope assertions to them regardless, the way `shipping`'s tests do (`seedOrder`, `seedShipment`), because a *second* test package can still land on any of these names in the future and nothing would stop it.

## What `TestRouteAccess` proves, and what it still cannot see

**Where you hit it:** an `/api/admin/` route no longer requires an admin
token — the `admin` group's middleware got dropped somewhere along the way —
and you want to know whether a test catches it. It does.

`TestRouteAccess` (`internal/server/routes_access_test.go`) drives the real
`NewRouter` and walks `allRoutes`, a hand-written table of 65
`method<TAB>path` lines, running a subtest per entry: an anonymous probe for every route, plus a real-token probe for
everything not named in the ten-line `publicRoutes` allowlist. A route whose
path starts with `/api/admin/` must 401 anonymously and 403 the non-admin
token; anything else not in `publicRoutes` must 401 anonymously and must not
403 the token; anything in `publicRoutes` must not 401 at all. Before this
test existed, `router_test.go` and `test/e2e` sampled the table and nothing
enumerated it.

**What it still cannot see:**

- **A route mounted in `router.go` and left out of `allRoutes`.** The table
  is hand-written, so an unlisted route is simply never probed and its auth
  class is never asserted. `web.Router` deliberately records nothing — it
  once exposed a `Routes()` accessor that made the table derivable, and that
  was removed because production never read it. The cost is this blind spot,
  which is the golden file's blind spot returning. Removal is
  now automatic and safe; removing is invisible.
- **Which rate limiter, if any, guards a route.** Its own entry above.
- **Anything about the handler behind the route.** A mounted route may answer
  any status once past the auth and admin checks; the test proves who is let
  through, never what happens after.

The rest of the picture is unchanged. A module's own `handler_test.go` builds
its own `web.NewRouter(mux).Group(...)` and writes the prefix itself rather
than importing anything from the real router — and it is not even the same
prefix: `internal/modules/category/adapter/http/admin_handler_test.go:375`
builds `web.NewRouter(mux).Group("/api/v1/admin")` while `NewRouter` mounts
the real admin group at `/api/admin`, with no `/v1` anywhere in production.
The test passes either way, because it never touches the real router.
`router_test.go` still samples rather than enumerates: 24 distinct `/api…`
paths appear in the whole file (`grep -oE '"/api[^"]*"'
internal/server/router_test.go | sort -u | wc -l`) against the 64 `/api`
routes, and several of those 24 only assert a 401 or a 403 rather than the
handler behind them.

**What decision 15 still costs:** nothing links the two halves. An
`adapter/http` method that no route mounts compiles, lints, passes
`check-boundaries`, and serves nothing, and no check says so. What decision 15
bought is that all 65 URLs are in one file — which is what makes
`TestRouteAccess` cheap enough to enumerate them at all.

## A repository write can leak outside its own transaction with no test failing

**Where you hit it:** a `Service`'s repository call moves outside its own
`tx.Run` callback — a bug that should fail a test — and nothing fails.

`testutil.FakeTxRunner.Run` (`internal/testutil/txrunner.go`) is `return
fn(ctx)`: it calls the callback inline, with no transaction underneath it, so
a mock-based `service_test.go` cannot observe whether a repository call
happened inside `tx.Run`'s closure or leaked outside it. Both look identical
to the fake. Five files are in that position (`grep -rl FakeTxRunner
--include='*_test.go' internal/modules`) — `cart`, `order`, `payment`,
`promotion` and `shipping`'s own `service_test.go` — which is exactly the five
modules that hold a `TxRunner`.

**What you would do:** a real `TxRunner` backed by a test transaction would let
a test assert call order across the boundary, at the cost of every affected
`service_test.go` needing a real Postgres connection instead of a mock —
trading a fast unit test for a slower, more honest one. Not done here.

## The composition site is deliberately tedious

**Where you hit it:** you open `internal/bootstrap/app.go` expecting the pile
of adapter aliases a template this size usually carries, and find seventeen —
`cartpg`, `categorypg`, `dashboardpg`, `inventorypg`, `notificationpg`,
`orderpg`, `paymentpg`, `productpg`, `promotionpg`, `reviewpg`, `shippingpg`,
`userpg`, `wishlistpg`, `userredis`, `channellog`, `notificationjobs` and
`paymentjobs`.

That is one alias per adapter package, not one per module: `auth`, `checkout`
and `money` own no store and cost none, `user` costs two. `func New` is 79
lines (`internal/bootstrap/app.go:64`-`142`) and builds every module in
dependency order — `inventory` first because `product` needs it, `order`
before `payment` because `payment`'s single `Orders` port takes the one
`*order.Service`, and `checkout` after both because it orchestrates them.

There is no `Deps` struct left to carry a raw `*pgxpool.Pool` field any more:
`New` builds one `database.DB{Primary, Replica}` value, threads it into every
`xxxpg.New(db)` call, and hands the resulting adapters to each module's `New`
as positional arguments. `payment` and `notification` used to reopen a
second pool each to build their own job-queue adapter; both now take an
`adapter/jobs.Queue` built from the same insert-only `river.Client`
(`queue.NewInsertClient(db)`, built once) and the same `database.DB` value
every other adapter gets: `notificationjobs.NewQueue(insertClient, db)` and
`paymentjobs.NewQueue(insertClient, db)`.

The tedium has moved three times, and the history is the interesting part.
Phase 0 made `bootstrap.New` the single composition root. Slicing moved most
of it back out, one level down, into each module's own `module.go` — and left
six aliases in `app.go` regardless (`ordercancel`, `ordercancelpg`,
`ordertransition`, `ordertransitionpg`, `orderquery`, `orderquerypg`), because
breaking the order/payment cycle needed three pieces of `order` built before
`order.New` itself could run. The flatten brought all of it back to `app.go`
and dissolved those six at the same time: `checkout` took the cycle, so
`order.New` runs first and hands `payment.New` one value for all three of its
order-facing ports.

`internal/server/router.go` keeps the other pile: 15 aliased `*http` imports,
one per module that serves a route, inside `NewRouter` itself. It
was 3 to 5 aliases in each of 14 files under `internal/transport/http/routes/`
before the collapse — 53 in total, plus one in the router file for the dev-only
mock gateway registrar. The honest description is that this pile has been
redistributed twice and never paid off. What each move bought is different:
decision 15 bought "every URL in one directory", and the flatten bought "every
URL in one file, in one function, in the order the router mounts them".

**What it costs beyond ugliness:** adding a module means touching `app.go`
once — one line to build it, one field on `App` — and `router.go` once. Adding
a route to an existing module touches the module's `adapter/http` for the
handler and `router.go` for the URL — `TestRouteAccess` picks the route up
from its path once its line is in `allRoutes`, so the only other hand-edit is a
`publicRoutes` line when the route must be public.
`New` carries no `//nolint:funlen`; `NewRouter` does, and its justification
says why — one flat wiring list mounting fifteen modules' routes, not nested
logic.

**What you would do:** leave it. Splitting `New` per module scatters the
wiring graph, and a single readable list of every module and what it depends
on is worth more than diff conflicts. If it becomes unbearable, split by
_layer_ (build every module's dependencies, then every module) not by module.

## RESOLVED: a dropped constructor argument used to be silently `nil`

**What it was.** `order.Deps` — before decision 17 collapsed its cart and inventory fields, and before `New` moved to positional parameters at all — held separate `InventoryReserve`, `InventoryDeduct` and `InventoryRestore` fields in a struct literal built by `internal/bootstrap/app.go`. Dropping one line from that literal, say `InventoryDeduct: inv`, compiled fine: a struct literal with a field left unset just leaves it at its zero value, so `order.Deps.InventoryDeduct` went silently `nil`, and `make check-boundaries`, `make lint`, `make test` and every `test/e2e` saga still passed, because the only method that called it — `finalizeFreeOrder`, reached only by a 100%-discount or entirely-free order — had no saga exercising it against the real wiring. This entry used to price exactly that gap, and flagged itself as due for a rewrite once `Deps` became positional parameters, on the chance that change closed it outright.

**Why it is closed.** It did. `order.New` now takes `inventory Inventory` (along with every other dependency) as a plain positional parameter, not a struct literal field, and `order.Service` holds one `inventory Inventory` field — the three-way split into `Reserve`/`Deduct`/`Restore` fields collapsed to one `Inventory` port under decision 17, before positional parameters removed the struct literal entirely. Omitting an argument from `order.New(orderpg.New(db), txRunner, logger, cartMod, inv, promotionMod, notificationMod)` is now `not enough arguments in call to order.New` — a build failure, not a silently zero-valued field. The specific gap this entry counted — a dropped dependency that compiles clean and only misbehaves against a rarely-exercised path — cannot occur through a constructor call any more, for any module, since none of the sixteen takes a `Deps` struct today.

**What remains.** Swapping two arguments of the *same* type at the same call site still compiles and still is not a bug a check would catch — that risk is [priced separately](#one-flat-service-satisfying-several-of-a-consumers-ports-leaves-the-compiler-nothing-to-check) and positional parameters do not touch it, since a same-typed transposition is legal in a positional call exactly as it was in a struct literal.

## Every keyset cursor depends on one unenforced date layout agreeing with itself

**Where you hit it:** a cursor a client sends back from a previous page decodes to a different instant than the one that produced it, and pagination silently returns the wrong rows — never an error, just a page that quietly skips or repeats.

`internal/platform/web/response/pagination_cursor.go` formats the timestamp half of a keyset cursor with one unexported constant, `cursorTimeFormat`, and every call site that builds a cursor for `CursorPage[T]` has to agree with it implicitly, by never formatting a cursor's timestamp any other way. Nothing enforces that agreement beyond there being exactly one constant and one function that reads it. A second encoder — a different endpoint hand-rolling its own cursor, a future refactor that inlines the format string somewhere convenient — would drift the moment it used a layout with different precision or a different timezone offset, and the failure mode is a mis-decoded timestamp, not a parse error: `time.Parse` with a mismatched-but-plausible layout can still succeed on the wrong value.

**What you would do:** keep every cursor-producing call site routed through `CursorPage[T]` rather than formatting a timestamp directly, and if a second encoder is ever genuinely needed, export `cursorTimeFormat` (or an equivalent constant) rather than letting a second literal format string exist anywhere in the tree.

## A duplicate product id in a stock-adjustment map is silently dropped, not summed

**Where you hit it:** you build a `map[uuid.UUID]int` for `inventory.Service.Reserve` / `inventory.Service.Deduct` / `inventory.Service.Restore` and write to the same product id twice while assembling it — `m[id] = 2`, then later `m[id] = 3` for the same `id`. No error, no sum. The map ends up holding `3`; the `2` is gone.

Every batch method — `Reserve`, `ReleaseBatch`, `Deduct`, `RestockBatch`, `Restore` — takes `map[uuid.UUID]int`. All five now live in one module — `Reserve`, `Deduct` and `Restore` on `inventory.Service`, `ReleaseBatch` and `RestockBatch` on the one `Repository` behind it — sharing one `buildStockValues` (`internal/modules/inventory/adapter/postgres/repository.go`) building its `VALUES` list straight off that map. A map holds one value per key, full stop, so there is nothing left to sum by the time this code runs. That is not the gap. The gap sits upstream of it: nothing stops the _construction_ of the map from writing the same key twice, and when that happens the second write overwrites the first with no signal at all — not a panic, not an error return, not a log line.

**Why it is safe today.** Every current caller builds the map from data that cannot contain a duplicate product id before this code ever runs:

- `cart_items` carries `UNIQUE (cart_id, product_id)`, and `cart.Service.Add` upserts via `ON CONFLICT (cart_id, product_id) DO UPDATE` — a cart cannot hold two rows for one product.
- `order.Service.Place` builds its reservation map one entry per cart-snapshot item, inheriting that guarantee directly — the map and the cart are keyed by the same product ids, one-for-one.
- `order_items` — read back by `order.Service`'s own `finalizeFreeOrder`, `cancelWithReversal` and `releaseOrderHolds`, and by payment's refund and charge-success paths via `order.Service.ListItemQuantities` (which is what `payment.Orders`'s `ListItemQuantities` method names; `bootstrap` hands `payment.New` the one `*order.Service` value it built first, so payment gets no second copy) — has **no unique constraint on `(order_id, product_id)`**, only `PRIMARY KEY (id)` and a plain index on `order_id` (`db/migrations/20260424120005_create_orders.sql`). It is unique-per-product today only because the one write path that populates it (`order.Service.Place` → `repo.CreateItems`, one row per cart-snapshot item) already can't produce duplicates. The invariant holds one level removed from any enforcement of its own.

**What it costs.** Nothing in `inventory`, nothing in the four call sites, and no database constraint enforces this. "No current caller can trigger it" is a fact about the callers, not about the map type — the type permits the duplicate write; it just resolves it wrong, silently, when one happens.

**What breaks it.** Any future path that inserts an `order_items` row without going through the cart snapshot — a bulk admin order-creation endpoint, an import job, a split-shipment line-item model (see multi-warehouse, above) — could write two rows for the same product on one order without violating any constraint that exists today. The next map built from that data drops one of the two quantities silently; inventory reserves or deducts less than the order actually needs. Nothing crashes, nothing logs. The order just ships short.

**What you would do:** put `UNIQUE (order_id, product_id)` on `order_items` — the better fix, since an order-line model without split-shipment support has no legitimate reason for two rows on one product, and a constraint stops the bad row from existing rather than checking for it after the fact every time it's read. A `len(map) == len(items)` assertion at each of the four call sites is the cheaper stopgap if you want it sooner than a migration — but do not add that guard speculatively ahead of a real caller that needs it. Today none does; a guard for a case nothing can reach is exactly the kind of code this refactor was removing.

## RESOLVED: `order.Snapshot` was one struct serving two different read shapes

**What it was.** `order/contract.Order` had eight fields (`ID`, `UserID`,
`Total`, `Status`, `CouponCode`, `StockDeducted`, `StockReversed`,
`Dispatched`) and two producer methods returning it off the same read:
`GetSnapshot`, payment's and checkout's, which filled all eight, and
`GetInfo`, shipping's ownership check, which filled only `ID`, `UserID` and
`Status` and left the other five at their zero values. Both returned the same
Go type, so nothing stopped a caller handed the sparse value from reading
`Dispatched` and always observing `false` — not because no order is
dispatched, but because `GetInfo` never set it.

**Why it is gone.** The flatten deleted `GetInfo` and kept one method,
`order.Service.Snapshot`, which fills every field. Shipping — the sparse
method's only caller — reads `Status` and `UserID`, both of which the full
projection populates from the same row, so the merge cost it nothing and
removed the class of bug outright. There is now one producer, one shape and
no subset to get wrong. The two-types fix this entry used to recommend
(`contract.Snapshot` plus `contract.Info`) is moot: one type with one
producer needs no split.

**What stayed open.** `Service.SettleRefund`
(`internal/modules/payment/service.go:501` and `:504`) branches on
`orderSnap.Dispatched` and `orderSnap.StockReversed`, and neither branch has
a test that sets either field `true`. That is an untested branch, not a
type-safety hole — the fields are populated now, so the branch reads what the
row actually says. It predates the flatten and is still worth a test.

## `RescueStuckJobsAfter` is client-wide, so a per-queue rescue window is not expressible

**The problem this section used to describe is resolved by construction, not by convention.**
`order.LoadConfig(paymentJobLease time.Duration)` used to validate a cross-module threshold
against a bare `time.Duration` parameter threaded in from `payment`'s own config, and nothing
type-enforced that the two `LoadConfig` calls ran in the right order or passed the right field
— a real incident followed when a shared worker-wide lease split into per-queue fields and both
guards kept validating a value the runner no longer used. That path is gone: `payment.LoadConfig`
validates `JobTimeout`, a field it owns, against `GatewayTimeout`, a field it also owns — no
parameter, no cross-module thread. The one guard that spans `order` and the worker,
`WORKER_RESCUE_AFTER` against `order.StaleProcessingThreshold`, is not in any `LoadConfig`
either: `internal/worker`'s `newClient` imports `order.StaleProcessingThreshold` as a
compile-time constant and compares it to `appCfg.Worker.RescueAfter` inline, before building the
`river.Client`. There is no load order to get wrong, because there is no second `LoadConfig`
call on this path.

**What is true instead:** River's `RescueStuckJobsAfter` (`WORKER_RESCUE_AFTER`) is one field on
`river.Config`, shared by every queue the client runs — `payment`, `notification` and `order`
alike (verified against `github.com/riverqueue/river@v0.45.0/client.go`: the field is declared
once, with no per-`QueueConfig` equivalent). Each worker's own `Timeout()` —
`PAYMENT_JOB_TIMEOUT`, `NOTIFICATION_JOB_TIMEOUT`, `ORDER_JOB_TIMEOUT` — is validated against
that queue's own constraints (payment's must be at least 3× its gateway timeout), but nothing
validates any of the three against `WORKER_RESCUE_AFTER` itself. River only checks its own
client-wide fallback `JobTimeout` against `RescueStuckJobsAfter` (`client.go`'s
`RescueStuckJobsAfter cannot be less than JobTimeout`), and this template never sets that
client-wide fallback, so the check passes trivially regardless of what any individual worker's
`Timeout()` is set to. Set `PAYMENT_JOB_TIMEOUT` above `WORKER_RESCUE_AFTER` and nothing stops
it at boot: a refund still genuinely running when the client-wide rescue window lapses gets
reclaimed and re-run out from under itself — the same double-execution risk
`payment.RefundWorker`'s idempotency guards exist to absorb, but for a config mistake this
template does not catch.

**What you would do.** Validate the missing invariant explicitly — either in `internal/worker`'s
`newClient` (it already has every value it needs in scope: `modCfg.Payment.JobTimeout`,
`modCfg.Notification.JobTimeout`, `modCfg.Order.JobTimeout` and `rescueAfter`, right where it
validates `rescueAfter` against `order.StaleProcessingThreshold` today), or by widening that
one function's check to all three. A per-queue rescue window is not a gap this template can
close by rearranging config: River does not expose one as of the version pinned in `go.mod`.

## `contract.go` can grow into the shared domain model `internal/shared/` was rejected for being

**Where you hit it:** a module's `contract.go` is read by every consumer of that module, so a field added there is public API. Nothing limits what may go in one, and the pressure is always to add "just one more field" rather than to ask why the consumer needs it. `db/OWNERSHIP.md`-style enforcement does not apply here: a struct field is not a table, so nothing machine-checks what `contract.go` may carry the way check 2 machine-checks table ownership. There used to be a partial guard — check 7 (`check_contract_leaf`) kept every `contract/` package to stdlib, `uuid` and `money`, so at least a published type could not drag a `domain/` along behind it. Decision 16 moved those types into the module's root package and check 7 was retired, so that half is gone too.

`order.Snapshot` is the shape this looks like from the inside: one struct with eight fields that two different producer methods once filled differently for two different consumers (see the resolved entry above). That happened inside a single phase, with both call sites known in advance. A `contract.go` accreting fields one unrelated PR at a time, each individually reasonable, is the same failure with no phase boundary forcing a review of the whole shape at once.

**What you would do:** before adding a field to a `contract.go`, check whether the consumer needs the _value_ or the _decision_. `order.Snapshot.Dispatched bool` is a decision `order` already made; a `Status string` field plus the consumer re-deriving "is this dispatched" from it would be the model leaking instead — the same distinction `ARCHITECTURE.md` §10 draws between `payment.OrderUpdater.MarkPaid` (an intent) and an ad-hoc from/to status list (the mechanics). `ARCHITECTURE.md` rejected `internal/shared/` for the same reason `contract.go` earns scrutiny: an owned, single-purpose surface with one publisher and named consumers stays legible; an unowned one that answers "what if we just add a field" enough times becomes a second copy of the schema with none of decision 6's ownership discipline. `internal/modules/money` is the one shared-kernel directory that survived that rejection, and it is worth watching for the same reason: a second value object landing beside it starts to look like `shared/` with a different parent.

## Context log attributes are write-only

**Where you hit it:** you want `request_id` in an error response body, or need to forward it as a header on an outbound call.

`logger.WithAttrs` stores a `[]slog.Attr` under an unexported key and only `ContextHandler.Handle` reads it. There is no accessor, and `middleware.GetRequestID` was deleted once nothing needed it. Both uses above need the value itself, not a log record.

**A second, sharper limit: nothing checks the single-naming invariant.** An attribute named at two points on one code path is emitted twice, and slog does not deduplicate keys. This used to have a job-side example: `Runner.processOne` named `job_id` for every job either queue drained, and any handler's own `Run` method that also called `logger.WithAttrs(ctx, slog.String("job_id", ...))` would have started emitting the key twice, with no test or linter to catch it. That edge is gone along with `internal/platform/jobs` — River jobs do not run through `logger.WithAttrs` at all now (decision 12), so there is one fewer place this invariant could quietly break. The two edges that remain, `middleware.RequestID` and `middleware.Auth`, are singular by construction: a request carries exactly one of each. The invariant itself is still unchecked in general, though — nothing stops a future edge from repeating a key an earlier one already named. The check is a grep of the callers, run by hand.

This is not hypothetical. Naming `user_id` at the auth edge immediately collided with an `invalidateStatusCache` helper that logged its own `user_id` — the user being acted upon, not the caller. On an admin role change the record carried both, and a last-wins parser kept the admin's id while silently dropping the target's. The fix was to rename the inner one to `target_user_id`, because the two values answer different questions. That helper was duplicated three times while `user` was sliced — one private copy per slice that needed it (`remove`, `adminupdate`, `updaterole`), each logging `target_user_id` the same way — and the collision this paragraph describes had to be fixed in each copy independently. One `invalidateStatusCache` method on `user.Service` (`internal/modules/user/service.go`) replaced all three, which is the shape of what decision 16 bought: three copies of one fix became one.

**What you would do:** for the accessor, re-add a typed one beside the middleware that produces the value — `middleware.GetRequestID` as it was, a `context.WithValue` next to the `logger.WithAttrs` call. Do not add a generic `logger.Attrs(ctx)` reader: callers would loop the slice matching on a string key, which worse than the typed accessor it replaced. For the naming invariant, grep the tree for a key before naming it at a new edge — a collision look like nothing at all until someone query the logs.

---

## When not to copy this

Structure above priced for one situation: system with several genuinely distinct domains, expected to live long enough that boundaries pay back, worked on by more than one person or agent. Outside that, overhead.

Do not copy this if:

- **You have one domain.** Sixteen module directories for one bounded context give you aliased imports, ports, mapper functions and an ownership check with nothing on the other side of the ledger. A single package with a `handler`/`service`/`repository` split is the right shape.
- **You need cross-module queries more often than not.** Every join across boundary become two queries and a port, and reporting is the _only_ carve-out. If most reads are aggregates over many entities, you want schema you can join, or read model from start — not dozen ownership fences and one exception.
- **You are prototyping.** Rules cost most at beginning, when boundaries still guesses. Ports declared around domain you not understand yet are wrong ports, and harder to move than functions.
- **You want to extract services soon.** This structure make _code_ side of extraction cheap and leave data side entirely undone — 16 cross-module foreign keys in one schema. If service split imminent, separate data first; module boundaries will follow more easily than reverse.
- **You need multi-currency or multi-warehouse on day one.** Both named above as redesigns. Starting from design that rejected them mean undoing ratified decisions before writing first feature.

Do copy it if you want boundaries checkable not aspirational, and willing to pay two queries, a mapper and an import alias for that. That the whole trade, and every section above a line item in it.

---

Read alongside:

- `ARCHITECTURE.md` — the nineteen decisions and fifteen rejections these are the shadow of. Decision 14 is marked reversed and decision 16 is what replaced it; the first six sections above are its bill.
- `db/OWNERSHIP.md` — table-ownership map, foreign-key inventory, and full blind-spot list for `make check-boundaries`.
- `AGENTS.md` — working rules, and which of them machine-checked.
