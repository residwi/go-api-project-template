# Limitations this architecture creates

`ARCHITECTURE.md` records twelve decisions and thirteen things this codebase
deliberately does not do. Every one of them bought something and charged for it.
This file is the invoice.

It exists because this repository is a **template**: the structure is the
product, and someone is about to copy it into a real system. A document that
only lists what a design makes easy teaches nothing, because the design was
never in danger of being blamed for those. What a reader actually needs is the
list of moments where they will hit a wall, so they can recognise the wall as
this design's and not their own mistake.

Each section below states the limitation, the moment you meet it, and what you
would have to do about it. Where a limitation is a decision's shadow, the
decision is cited rather than restated — read them together.

The last section, [When not to copy this](#when-not-to-copy-this), is for
someone still deciding.

---

## You cannot filter or sort a product listing by stock

**Where you hit it:** you try to add `?in_stock=true` to `GET /api/products`.

`products` is owned by `product` and `inventory_levels` is owned by `inventory`
(decision 6, decision 7), so the listing query cannot join them. The port is
`product.InventoryReader.GetAvailability(ctx, ids)` — batch-shaped, and asked
*after* the page has already been selected. `product.Service.ListPublished`
calls `repo.ListPublished` and then `enrich`, in that order, and it cannot be
the other order: enrich needs the ids the page chose.

So the filter can only be applied to rows already fetched, and that breaks the
pagination rather than merely slowing it. `ListPublished` is keyset-paginated on
`(created_at, id)` and fetches `Limit + 1` rows to decide `hasMore`. Ask for 20,
drop the 8 that are out of stock, and you have a page of 12 whose cursor claims
the client stopped at row 20. Repeat, and page sizes wobble unpredictably while
`hasMore` lies. Sorting by stock is worse: the sort key is not in the table the
`ORDER BY` runs against, so there is nothing to sort on until after the window
has been chosen.

Today's filters — `category_id`, `min_price`, `max_price`, `search` — all work
because they are columns of `products`.

**There is no partial fix.** Fetching extra rows to compensate is not one: you
cannot know how many extra, and the cursor still has to name a row you did not
return.

**What you would do:** build the read model `ARCHITECTURE.md` rejects for now.

```text
product_view
  product_id, name, slug, price, currency, status, category_id, available_stock
```

`inventory` writes it on every level change — in the same transaction if you
want it exact, asynchronously if you accept lag — and `GET /api/products` reads one
table it can filter, sort and paginate on freely. That is a new table, a new
writer, and a decision about staleness. It is a feature, and it is the right
one the day a storefront needs it. It is left out because a read model with no
consumer is speculative infrastructure.

## Multi-warehouse is a redesign, not a column

**Where you hit it:** a second warehouse appears and you reach for
`ALTER TABLE inventory_levels ADD COLUMN warehouse_id`.

`inventory_levels` is `PRIMARY KEY (product_id)`. That was ratified, not
overlooked — the migration says so in a comment, and `ARCHITECTURE.md` lists
multi-warehouse under Rejected. The single-row-per-product invariant is load
bearing in four places, and adding the column breaks all four silently:

1. `ReserveBatch` updates `WHERE i.product_id = v.product_id AND
   i.available_stock >= v.qty`. With several rows per product, one 1-unit
   reserve increments `reserved_stock` in *every* warehouse row.
2. `if int(tag.RowsAffected()) != len(ids)` is the insufficient-stock signal.
   Once rows and products are not the same count, that comparison stops meaning
   anything — and it fails open or closed depending on how many warehouses
   happen to stock the item.
3. Availability stops being a row predicate. `available_stock >= qty` becomes
   `SUM(available_stock) >= qty GROUP BY product_id`, which no single guarded
   `UPDATE` can enforce without first deciding which warehouse each unit comes
   out of.
4. Deadlock avoidance rests on `SELECT 1 FROM inventory_levels WHERE product_id
   = ANY($1) ORDER BY product_id FOR UPDATE`. A composite key needs a composite
   lock order, and the argument for why concurrent checkouts cannot deadlock
   stops being obvious enough to trust.

Outside `inventory` it also needs an allocation policy (fill-first? nearest?
split the line?), `warehouse_id` on `order_items` so a release or deduction
targets the row the units came from, per-line reservation *records* rather than
counters so a refund can return units where they came from, and split-shipment
support in `shipping`.

**What you would do:** plan it as a feature touching four modules and adding a
table. What you must not do is add a nullable `warehouse_id` that every query
ignores — that is worse than its absence, because the next reader will assume it
works.

## Two queries where one join would do

**Where you hit it:** you read `product.Service.ListPublished` and count round
trips.

Every product listing and every single-product read costs a second query to
`inventory` for the same ids. That is the deliberate price of decision 6.

The trade is bounded, which is the only reason it is acceptable: one extra query
per *page*, not per row, and it does not grow with page size. The port is
batch-shaped (`GetAvailability(ctx, ids)`) specifically so the N+1 version is
awkward to write.

**If you ever see one inventory call per product, that is a bug, not the
design.** It will look like a `for` loop around a single-id lookup.

**What you would do:** nothing, until profiling says otherwise; then the read
model above collapses it back to one query.

## Creating a sellable product takes two admin calls

**Where you hit it:** you `POST /api/admin/products` and the product has no stock.

`product` may not write `inventory_levels`, so the create path can only ask
`inventory` to materialise a row via `product.InventoryRegistrar.EnsureLevel`.
Setting an actual quantity is a separate call to `inventory`. There is no
`stock_quantity` field on product's write DTOs — it was removed, and its absence
is the point.

The alternative is `product` writing inventory's table inside its own
transaction, which is precisely the violation decision 6 exists to remove.

**What you would do:** accept it, and make the client do both calls. If
one-call creation matters, that is an orchestration concern and belongs in a
caller that holds both ports — not in `product`.

## The cart is not a quote

**Where you hit it:** a price changes between a customer adding an item and
checking out, and the cart total changes under them.

`cart_items` has columns `id, cart_id, product_id, quantity, created_at,
updated_at`. There is **no price column**. `cart.Service.GetCart` reads the
lines and then calls `products.GetByIDs` for the current name, price, status and
available stock. So the cart does not display a stale price — it never pinned
one at all. Every read is current.

Note this cuts against the intuitive framing. The cart's total is never
*stale*; it is **unpinned**. What is genuinely a point-in-time snapshot is the
`available_stock` each line reports: it was true when the cart was read and can
be wrong by the time `PlaceOrder` runs, which is why checkout re-reserves
against `inventory` rather than trusting what the cart displayed.

`order` does snapshot prices at placement, so an *order* is internally
consistent forever. The cart is not, and is not meant to be.

**What you would do:** if you need a held price, add `unit_price` and `currency`
to `cart_items`, write them at add-time, and then decide the thing that actually
makes this hard — what happens when the held price and the current price
disagree at checkout. Reprice silently? Refuse? Show both? A price column
without an answer to that question just moves the surprise later.

## An unsellable cart line is shown, not hidden

**Where you hit it:** a product is archived, unpublished or deleted after a
customer added it, and `GET /api/cart` still returns the line — with
`"sellable": false`, and excluded from `total`.

This is a behaviour change, and it was chosen. The previous implementation
dropped the line with `JOIN … AND p.deleted_at IS NULL`, so the customer's total
fell with nothing on screen to explain it. If the product record is gone
entirely, `cart.Service.GetCart` substitutes a synthetic
`&Product{Status: "unavailable"}` placeholder rather than dropping the item.

**What it costs:** every client rendering a cart must handle `sellable: false`,
and a client written against the old behaviour will show a line it cannot check
out. `Cart.Total()` folds sellable lines only, so `total` will not equal the sum
of the line prices the client can see — which looks like a bug if you have not
read this.

**What you would do:** render unsellable lines distinctly and offer a remove
action. Do not re-add the `JOIN` filter.

## A mixed-currency cart is a 400 from `GET /api/cart`

**Where you hit it:** a cart contains items priced in different currencies, and
the endpoint that used to return 200 now returns 400.

`money.Money.Add` refuses to sum across currencies, so `cart.Cart.Total()`
returns `(money.Money, error)`, and `internal/modules/cart/http/handler.go`'s
`GetCart` propagates that error rather than publishing a total it could not
compute. The error wraps
`apperror.ErrBadRequest` alongside `money.ErrCurrencyMismatch`, because
`ErrCurrencyMismatch` on its own matches no case in `response.HandleErr` and
would surface as a 500 for what is plainly user input.

**Nothing prevents such a cart existing.** Prices are per-product and
`cart.AddItem` does not constrain them, so a catalogue with mixed currencies
will produce this. Checkout already rejected it; this change makes
`GET /api/cart` agree with `PlaceOrder` instead of showing a number denominated
in nothing.

Two things soften it and both are deliberate: `Total()` folds **sellable lines
only**, so an archived line in a foreign currency still yields a clean 200; and
an empty cart yields `total: 0` rather than an error.

**What you would do:** decide currency at a level above the product. Constrain
the catalogue to one currency, or scope carts by currency and reject the add
rather than the read. The 400 is the correct report of an inconsistent cart; it
is not the place to fix it.

## `promotion` and `dashboard` amounts are plain `int64`

**Where you hit it:** you reach for `money.Money` in `promotion` or `dashboard`
and find nothing to construct it from. Neither package references `money` at
all.

`money.Money` covers exactly four features — `order`, `payment`, `product`,
`cart`. `ARCHITECTURE.md` §10 states why the other two are out, and the reasons
are load-bearing rather than bookkeeping:

- `Promotion.Value int64` is **polymorphic**. With `Type == TypePercentage` it
  is a percentage; with `TypeFixedAmount` it is minor units. `money.New(10,
  "USD")` to mean "10%" would be a value object asserting something false.
  `promotion` also has no currency field anywhere, so its genuinely monetary
  `MinOrderAmount`, `MaxDiscount` and `CouponUsage.Discount` have nothing to
  pair with.
- `dashboard` aggregates revenue across all orders and has no currency field, so
  any currency it named would be a guess.

**What it costs:** the seam between `order` and `promotion` is untyped in the
middle. `order.CouponReserver.Reserve` passes `orderSubtotal int64` and returns
`discountAmount int64`, and `order/service.go` re-pairs it with
`money.New(discount, subtotal.Currency)` on its own side. That is also where the
clamp lives — `max(subtotal-discount, 0)` as plain arithmetic, because
`Money.Sub` deliberately does not decide whether money may go negative. If you
add a multi-currency promotion system, this seam is where the type safety is
missing and where a wrong-currency discount would pass unnoticed.

**What you would do:** give `promotion` a currency column and split `Value` into
two fields — a percentage and a `Money` — before trying to use `Money` across
that port. Retrofitting `Money` onto a polymorphic column first will produce a
type that lies.

## Extracting a module into a service is a data migration, not a refactor

**Where you hit it:** you decide to pull `order` out into its own service and
discover the Go part is the easy half.

The code boundaries are genuinely clean — no module imports another, and every
cross-module call goes through an interface the *consumer* declared, so each
module's port list is already the API its service would expose. The database is
one schema with **25 foreign keys, 18 of which cross a module boundary**
(measured against `pg_constraint` on a migrated database; the full list and the
7 internal ones are in `db/OWNERSHIP.md`).

Those 18 are exactly what makes the split a data problem. You cannot put
`orders` and `products` in separate databases while `order_items.product_id`
carries a foreign key. Step one of any extraction is dropping 18 constraints and
re-expressing each as an application-level check *with an explicit answer for
the race the constraint used to close* — because a port checks at a different
moment than the one the write commits in, and the constraint has no such window.
That is a migration with a correctness argument attached, not a refactor.

One of the 18 is load-bearing in Go rather than merely defensive.
`products.category_id` is the only cross-module constraint that can fire in
normal operation, because `categories` is the only hard-deleted table another
module references; `category`'s adapter catches the violation as a backstop
behind `category.ProductCounter`'s friendlier pre-check. Drop that constraint
and the backstop goes with it.

**What you would do:** budget the data split as its own project. Every port
declared in this refactor makes the code side cheap; none of them touch this
side.

## Foreign-key fan-in is not the dependency graph

**Where you hit it:** you try to work out which module to extract first by
looking at which tables everything references, and get an answer that is nearly
inverted.

| Module | Inbound FKs | Inbound ports |
| --- | --- | --- |
| `user` | 7 | **1** |
| `order` | 6 | **7** |
| `product` | 6 | **2** |
| `inventory` | **0** | **5** |
| `category` | 2 | 0 |

Inbound FKs count constraints referencing a table the module owns. Inbound ports
count interfaces *other* modules declare that this module's service satisfies —
`auth.UserProvider`, `payment.OrderGetter`, `product.InventoryReader` and so on.

`users` is the most-referenced table in the schema and almost nothing calls into
`user`: seven tables carry a `user_id`, and a caller writing one already **has**
the id, so it has nothing to ask. `inventory_levels` has no inbound foreign keys
whatsoever and five interfaces across three modules declare ports against
`inventory`, because stock is
an answer that *changes* and must be asked for every time.

**Foreign-key fan-in measures how many tables carry an identity. Port fan-in
measures how much behaviour other modules need.** They are close to independent,
and neither alone tells you what coupling costs.

`orders` is the only table high on both. That — not its FK count — is the real
argument that `order` is the hardest module to extract and the one to be most
careful modifying.

**What you would do:** when planning an extraction, count ports first and
constraints second. Ports tell you how much runtime coupling you must replace
with network calls; constraints tell you how much of the data migration you
must argue for.

## `make check-boundaries` has blind spots, and they are where you would hide

**Where you hit it:** you assume a green `Boundaries OK` means the boundaries
hold. It means the boundaries hold *in the places the script can see*.

The check is three greps, not a compiler. `db/OWNERSHIP.md` documents the gaps
in full; the ones most likely to bite:

- **Table names must be literals.** The check greps for the identifier after
  `FROM` / `JOIN` / `INSERT INTO` / `UPDATE` / `TRUNCATE` / `COPY`. Every query
  today has its table name in the string literal, but `fmt.Sprintf` is already
  routine in these adapters — `product/postgres`, `order/postgres` and six
  others build `WHERE` fragments and placeholder lists that way. The habit of
  assembling SQL exists; it simply has not reached a table name yet. The day it
  does, the check goes quiet rather than failing. `pgx.CopyFrom` would be the
  same hole immediately: its table is a `pgx.Identifier` Go value with no
  keyword in front of it. Nothing uses it today.
- **Prose in a production string literal is a false positive.** Comments and
  `_test.go` files are excluded, but `"update orders failed"` in a `postgres`
  package still reports `orders`. It fails loudly rather than quietly, which is
  the right direction, but it is the failure mode that gets a check disabled.
- **Test files are skipped, deliberately.** A test seeds sibling tables to
  satisfy foreign keys, and that is fixture setup, not an architectural
  crossing. The cost is that a real violation living in a `_test.go` helper is
  invisible.
- **`dashboard` is exempt wholesale.** Nothing verifies it stays read-only —
  no grant, no separate role, no check. An `UPDATE` in
  `internal/modules/dashboard/postgres` passes CI today. Nothing bounds it to its
  current two tables either, so "may read anything" can become a second,
  undeclared copy of the schema without a review comment.
- **Only `internal/<module>/postgres/` is scanned.** A stray query in a service
  file, in `db/seeds/data.sql`, or inside a migration is not. The whole subtree
  of that directory is scanned, though, so a `postgres/queries/` subpackage is
  not a way out.
- **Ownership is per table, so column coupling is invisible.** `dashboard`
  depends on `order_items.unit_price`. Rename that column and `dashboard`
  breaks at runtime, in a query only an admin runs, with `go build` unable to
  see inside a SQL string.
- **Nothing the database does on its own.** A view, trigger or function
  crossing modules is not Go source and is not scanned.

**What you would do:** treat the check as a ratchet against regression, not
proof of correctness. If you start building table names dynamically, replace the
grep with a parser or ban the pattern — do not widen the allowlist.

## The read-replica seam is built, configured, and unused

**Where you hit it:** you set `READER_DATABASE_URL`, expect read load to move,
and nothing changes.

Everything except the last link exists. `READER_DATABASE_URL` is in
`.env.example` and in README's environment table.
`database.NewReaderPostgres` builds a second pool. `transport/http/server.go`
constructs it at boot and stores it as `Deps.ReaderPool`.
`database.ReadDB(ctx, primary, reader)` picks the reader unless the request has
been marked by `database.WithRecentWrite`.

**No repository calls it.** Every adapter constructor is `New(pool
*pgxpool.Pool)` — one pool — and `Deps.ReaderPool` is read by nothing outside
`server.go`. Grep for `ReadDB` outside `internal/platform/database` and you find
only that package's own tests. `WithRecentWrite` has no production caller
either.

So setting the variable opens a connection pool to a replica that receives no
queries. This is the failure mode `ARCHITECTURE.md` warns about under
multi-warehouse — a knob that looks wired and is not — in a different place.

**What you would do:** either finish it or remove it. Finishing it means each
`postgres` adapter taking both pools and routing genuinely read-only methods
through `ReadDB`, plus deciding where `WithRecentWrite` is applied (a middleware
after any non-GET, most likely) so a read-your-own-write does not hit a lagging
replica. Removing it means deleting the config field, the pool, the `Deps`
field, and the two helpers. Leaving it as it is means the next person to hit a
read-throughput problem will believe they already have a replica.

## The charge job is dispatched but never enqueued

**Where you hit it:** you read `payment.Service.Process`, see it switch on
`job.Action` with a `case ActionCharge: return s.processChargeJob(...)`, and
reasonably conclude charges run through the job queue like refunds do. They do
not. **No production code ever creates a `payment_jobs` row with
`action='charge'`** — all three `CreateJob` call sites in `payment/service.go`
enqueue `ActionRefund`. So `processChargeJob` is unreachable outside tests.

**Why it looks otherwise.** Charging happens inline instead, on two paths:
`InitiatePayment` finalises synchronously when the gateway captures funds
immediately, and the webhook finalises when it does not. Both call
`FinalizePaymentSuccess` with a **synthetic** `Job` carrying only `PaymentID`,
`OrderID` and `Action` — no `ID`, because there is no row. Three consequences
that are individually invisible:

- `MarkJobCompleted(job.ID)` inside `FinalizePaymentSuccess` runs
  `WHERE id = '00000000-0000-0000-0000-000000000000'` for those two callers. It
  is a deliberate no-op, not a lost write — but `MarkJobCompleted` discards its
  rows-affected count, so nothing at runtime distinguishes that from success.
- The webhook's follow-up `MarkJobCompletedByPaymentID(p.ID, ActionCharge)` also
  matches zero rows, always.
- A test asserting "no pending charge job remains" passes whether or not the
  bookkeeping runs, because the count is zero either way.
  `test/e2e/fulfillment_failed_test.go` says so in a comment rather than
  implying coverage it does not have.

**What you would do about it.** If charges should be queued — the honest reading
of `Process` — then `InitiatePayment` needs to enqueue an `ActionCharge` job and
the inline finalisation becomes the worker's job, which also gets you retry and
backoff for free on the most failure-prone call in the system. If they should
not, delete `processChargeJob`, the `ActionCharge` case, and the two
`MarkJobCompleted*` calls that can only ever match nothing. Either is a
half-day. What costs more is the current state, where a reader has to trace three
call sites to discover that a queue they can see is not used.

This is the same shape as the read-replica seam above: a mechanism that exists,
compiles, is dispatched, and never runs.

## The test suite shares one Postgres and one Redis, and slots are hand-assigned

**Where you hit it:** you add a test package, copy a `TestMain` from a
neighbour, and another package's tests start failing intermittently.

`internal/testhelper` starts two long-lived containers by fixed name —
`go-api-test-postgres` and `go-api-test-redis` — and every test binary attaches
to whichever already exists. Isolation is by **claimed slot**, not by container:

- **Postgres: one database per package.** `MustStartPostgres(dbName)` does
  `DROP DATABASE IF EXISTS <name> WITH (FORCE)` then `CREATE DATABASE`, then
  runs the migrations. Two packages passing the same name will drop each
  other's database mid-run — `WITH (FORCE)` terminates the other backend, so it
  is not even a polite failure.
- **Redis: a hand-assigned integer.** `MustStartRedis(dbIndex)` takes the index
  from a comment block in `internal/testhelper/testhelper.go`. Indices 0–5 are
  claimed today (`platform/cache`, `transport/http/middleware`, `modules/user/postgres`,
  `transport/http`, `modules/user`, `test/e2e`). `ResetRedis` calls `FlushDB`, so
  reusing an index flushes the other package's fixtures.

Nothing enforces either claim. A duplicate name or index compiles, passes
review, and fails as a flake in an unrelated package — the worst possible
signal, because the failure is nowhere near the change.

There are two further consequences worth knowing before writing tests:

- **`t.Parallel()` does not buy anything within a package**, because subtests
  share one database. Parallelism comes from `go test` running package binaries
  concurrently, which is exactly why integration tests stay colocated rather
  than collapsing into one `test/integration` package (decision 11).
- **`make test` cannot run without Docker.** There are no build tags and no
  short mode. Every package that touches Postgres or Redis fails outright.

**What you would do:** when adding a test package, claim a database name and (if
it needs Redis) the next free index, and update the registry comment in
`internal/testhelper/testhelper.go` in the same commit. If the suite grows much
past 15 Redis-using packages, the index space runs out and the allocation has to
become dynamic.

## The composition site is deliberately tedious

**Where you hit it:** you open `internal/transport/http/router.go` and find 27
aliased adapter imports (`cartpg`, `carthttp`, `orderpg`, …) above a 99-line
`NewRouter`.

There are 13 packages named `postgres` and 14 feature packages named `http`, so
every site that wires them needs aliases. `cmd/worker/main.go` needs 7 of its
own. `ARCHITECTURE.md` §0 and §3 own this: in a product codebase the subpackage
split would be hard to justify, and here it is the point — a physical boundary
makes `payment` importing `payment/postgres` a compile error rather than a
convention.

**What it costs beyond ugliness:** adding a feature means touching one long
function that every other feature also touches, so feature branches collide
there. `NewRouter` carries a `//nolint:funlen` with a stated reason, and the
cost is concentrated in one file on purpose — but it is still concentrated in
*one* file.

**What you would do:** leave it. Splitting `NewRouter` per feature scatters the
route table, and the single readable list of every route in the system is worth
more than the diff conflicts. If it becomes unbearable, split by *layer* (build
all repositories, then all services, then all routes) rather than by feature.

---

## When not to copy this

The structure above is priced for one situation: a system with several genuinely
distinct domains, expected to live long enough that boundaries pay back, worked
on by more than one person or agent. Outside that, it is overhead.

Do not copy this if:

- **You have one domain.** Fourteen modules for one bounded context gives you
  the aliased imports, the ports, the mapper functions and the ownership check
  with nothing on the other side of the ledger. A single package with a
  `handler`/`service`/`repository` split is the right shape.
- **You need cross-module queries more often than not.** Every join across a
  boundary becomes two queries and a port, and reporting is the *only*
  carve-out. If most of your reads are aggregates over many entities, you want
  a schema you can join, or a read model from the start — not a dozen ownership
  fences and one exception.
- **You are prototyping.** The rules cost most at the beginning, when the
  boundaries are still guesses. Ports declared around a domain you do not
  understand yet are wrong ports, and they are harder to move than functions.
- **You want to extract services soon.** This structure makes the *code* side of
  extraction cheap and leaves the data side entirely undone — 18 cross-module
  foreign keys in one schema. If a service split is imminent, separate the data
  first; the module boundaries will follow more easily than the reverse.
- **You need multi-currency or multi-warehouse on day one.** Both are named
  above as redesigns. Starting from a design that rejected them means undoing
  ratified decisions before writing your first feature.

Do copy it if you want the boundaries to be checkable rather than aspirational,
and you are willing to pay two queries, a mapper and an import alias for that.
That is the whole trade, and every section above is a line item in it.

---

Read alongside:

- `ARCHITECTURE.md` — the twelve decisions and thirteen rejections these are the
  shadow of.
- `db/OWNERSHIP.md` — the table-ownership map, the foreign-key inventory, and
  the full blind-spot list for `make check-boundaries`.
- `AGENTS.md` — the working rules, and which of them are machine-checked.
