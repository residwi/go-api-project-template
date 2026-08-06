# Limitations this architecture creates

`ARCHITECTURE.md` record fourteen decisions and fifteen things this codebase deliberately not do. Every one bought something and charged for it. This file the invoice.

Exist because this repo a **template**: structure is product, someone about to copy into real system. Doc listing only what design make easy teach nothing — design never in danger of blame there. Reader need list of moments where they hit wall, so they recognize wall as this design's, not own mistake.

Each section state limitation, moment you meet it, what you must do about it. Where limitation is decision's shadow, decision cited not restated — read together.

Last section, [When not to copy this](#when-not-to-copy-this), for someone still deciding.

---

## You cannot filter or sort a product listing by stock

**Where you hit it:** you try add `?in_stock=true` to `GET /api/products`.

`products` owned by `product`, `inventory_levels` owned by `inventory` (decision 6, decision 7), so listing query cannot join them. Port is `product.InventoryReader.GetAvailability(ctx, ids)` — batch-shaped, asked _after_ page already selected. `product.Service.ListPublished` call `repo.ListPublished` then `enrich`, that order, cannot be other order: enrich need ids page chose.

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

1. `ReserveBatch` update `WHERE i.product_id = v.product_id AND
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

`product` may not write `inventory_levels`, so create path can only ask `inventory` to materialise row via `product.InventoryRegistrar.EnsureLevel`. Setting actual quantity is separate call to `inventory`. No `stock_quantity` field on product's write DTOs — removed, and absence is the point.

Alternative is `product` writing inventory's table inside own transaction — precisely the violation decision 6 exist to remove.

**What you would do:** accept it, make client do both calls. If one-call creation matter, that orchestration concern, belong in caller holding both ports — not in `product`.

## The cart is not a quote

**Where you hit it:** price change between customer adding item and checking out, cart total change under them.

`cart_items` have columns `id, cart_id, product_id, quantity, created_at,
updated_at`. **No price column**. `cart.Service.GetCart` read lines then call `products.GetByIDs` for current name, price, status, available stock. So cart not display stale price — never pinned one at all. Every read current.

Note this cut against intuitive framing. Cart's total never _stale_; it **unpinned**. What genuinely point-in-time snapshot is `available_stock` each line report: true when cart read, can be wrong by time `PlaceOrder` run — why checkout re-reserve against `inventory` instead of trusting what cart displayed.

`order` do snapshot prices at placement, so an _order_ internally consistent forever. Cart not, and not meant to be.

**What you would do:** if need held price, add `unit_price` and `currency` to `cart_items`, write them at add-time, then decide the thing that actually make this hard — what happen when held price and current price disagree at checkout. Reprice silently? Refuse? Show both? Price column without answer to that question just move surprise later.

## An unsellable cart line is shown, not hidden

**Where you hit it:** product archived, unpublished or deleted after customer added it, and `GET /api/cart` still return line — with `"sellable": false`, excluded from `total`.

Behaviour change, and chosen. Previous implementation drop line with `JOIN … AND p.deleted_at IS NULL`, so customer's total fell with nothing on screen to explain. If product record gone entirely, `cart.Service.GetCart` substitute synthetic `&Product{Status: "unavailable"}` placeholder instead of dropping item.

**What it costs:** every client rendering cart must handle `sellable: false`, and client written against old behaviour will show line it cannot check out. `Cart.Total()` fold sellable lines only, so `total` will not equal sum of line prices client can see — look like bug if you not read this.

**What you would do:** render unsellable lines distinctly, offer remove action. Do not re-add the `JOIN` filter.

## A mixed-currency cart is a 400 from `GET /api/cart`

**Where you hit it:** cart contain items priced in different currencies, endpoint that used to return 200 now return 400.

`money.Money.Add` refuse to sum across currencies, so `cart.Cart.Total()` return `(money.Money, error)`, and `internal/modules/cart/http/handler.go`'s `GetCart` propagate that error instead of publishing total it could not compute. Error wrap `apperror.ErrBadRequest` alongside `money.ErrCurrencyMismatch`, because `ErrCurrencyMismatch` alone match no case in `response.HandleErr` and would surface as 500 for what is plainly user input.

**Nothing prevents such a cart existing.** Prices per-product and `cart.AddItem` not constrain them, so catalogue with mixed currencies will produce this. Checkout already reject it; this change make `GET /api/cart` agree with `PlaceOrder` instead of showing number denominated in nothing.

Two things soften it, both deliberate: `Total()` fold **sellable lines only**, so archived line in foreign currency still yield clean 200; and empty cart yield `total: 0` not error.

**What you would do:** decide currency at level above product. Constrain catalogue to one currency, or scope carts by currency and reject the add not the read. The 400 is correct report of inconsistent cart; not the place to fix it.

## `promotion` and `dashboard` amounts are plain `int64`

**Where you hit it:** you reach for `money.Money` in `promotion` or `dashboard` and find nothing to construct it from. Neither package reference `money` at all.

`money.Money` cover exactly four features — `order`, `payment`, `product`, `cart`. `ARCHITECTURE.md` §10 state why other two are out, and reasons load-bearing not bookkeeping:

- `Promotion.Value int64` is **polymorphic**. With `Type == TypePercentage` it a percentage; with `TypeFixedAmount` it minor units. `money.New(10,
"USD")` to mean "10%" would be value object asserting something false. `promotion` also have no currency field anywhere, so its genuinely monetary `MinOrderAmount`, `MaxDiscount` and `CouponUsage.Discount` have nothing to pair with.
- `dashboard` aggregate revenue across all orders and have no currency field, so any currency it named would be guess.

**What it costs:** seam between `order` and `promotion` untyped in middle. `order.CouponReserver.Reserve` pass `orderSubtotal int64` and return `discountAmount int64`, and `order/service.go` re-pair it with `money.New(discount, subtotal.Currency)` on own side. That also where clamp live — `max(subtotal-discount, 0)` as plain arithmetic, because `Money.Sub` deliberately not decide whether money may go negative. If you add multi-currency promotion system, this seam where type safety missing and where wrong-currency discount would pass unnoticed.

**What you would do:** give `promotion` a currency column and split `Value` into two fields — a percentage and a `Money` — before trying to use `Money` across that port. Retrofitting `Money` onto polymorphic column first will produce type that lies.

## Extracting a module into a service is a data migration, not a refactor

**Where you hit it:** you decide to pull `order` out into own service and discover Go part is easy half.

Code boundaries genuinely clean — no module import another, every cross-module call go through interface the _consumer_ declared, so each module's port list already the API its service would expose. Database is one schema with **25 foreign keys, 18 of which cross a module boundary** (measured against `pg_constraint` on migrated database; full list and the 7 internal ones in `db/OWNERSHIP.md`).

Those 18 exactly what make split a data problem. Cannot put `orders` and `products` in separate databases while `order_items.product_id` carry foreign key. Step one of any extraction is dropping 18 constraints and re-expressing each as application-level check _with explicit answer for the race the constraint used to close_ — because port check at different moment than the one the write commits in, and constraint have no such window. That a migration with correctness argument attached, not a refactor.

One of the 18 load-bearing in Go not merely defensive. `products.category_id` is only cross-module constraint that can fire in normal operation, because `categories` is only hard-deleted table another module reference; `category`'s adapter catch violation as backstop behind `category.ProductCounter`'s friendlier pre-check. Drop that constraint and backstop go with it.

**What you would do:** budget data split as own project. Every port declared in this refactor make code side cheap; none of them touch this side.

## Foreign-key fan-in is not the dependency graph

**Where you hit it:** you try work out which module to extract first by looking at which tables everything reference, and get answer nearly inverted.

| Module      | Inbound FKs | Inbound ports |
| ----------- | ----------- | ------------- |
| `user`      | 7           | **1**         |
| `order`     | 6           | **7**         |
| `product`   | 6           | **2**         |
| `inventory` | **0**       | **5**         |
| `category`  | 2           | 0             |

Inbound FKs count constraints referencing table the module owns. Inbound ports count interfaces _other_ modules declare that this module's service satisfies — `auth.UserProvider`, `payment.OrderGetter`, `product.InventoryReader` and so on.

`users` most-referenced table in schema and almost nothing call into `user`: seven tables carry `user_id`, and caller writing one already **has** the id, so nothing to ask. `inventory_levels` have no inbound foreign keys whatsoever and five interfaces across three modules declare ports against `inventory`, because stock is answer that _changes_ and must be asked every time.

**Foreign-key fan-in measures how many tables carry an identity. Port fan-in
measures how much behaviour other modules need.** Close to independent, and neither alone tell you what coupling costs.

`orders` only table high on both. That — not its FK count — the real argument `order` is hardest module to extract and the one to modify most carefully.

**What you would do:** when planning extraction, count ports first, constraints second. Ports tell how much runtime coupling you must replace with network calls; constraints tell how much of data migration you must argue for.

## `make check-boundaries` has blind spots, and they are where you would hide

**Where you hit it:** you assume green `Boundaries OK` mean boundaries hold. It mean boundaries hold _in the places the script can see_.

Check is three greps, not compiler. `db/OWNERSHIP.md` document gaps in full; ones most likely to bite:

- **Table names must be literals.** Check grep for identifier after `FROM` / `JOIN` / `INSERT INTO` / `UPDATE` / `TRUNCATE` / `COPY`. Every query today have table name in string literal, but `fmt.Sprintf` already routine in these adapters — `product/postgres`, `order/postgres` and six others build `WHERE` fragments and placeholder lists that way. Habit of assembling SQL exist; simply not reached table name yet. Day it does, check go quiet instead of failing. `pgx.CopyFrom` would be same hole immediately: its table a `pgx.Identifier` Go value with no keyword in front. Nothing use it today.
- **Prose in a production string literal is a false positive.** Comments and `_test.go` files excluded, but `"update orders failed"` in `postgres` package still report `orders`. Fail loudly not quietly — right direction, but the failure mode that get check disabled.
- **Test files are skipped, deliberately.** Test seed sibling tables to satisfy foreign keys — fixture setup, not architectural crossing. Cost: real violation living in `_test.go` helper is invisible.
- **`dashboard` is exempt wholesale.** Nothing verify it stay read-only — no grant, no separate role, no check. `UPDATE` in `internal/modules/dashboard/postgres` pass CI today. Nothing bound it to current two tables either, so "may read anything" can become second, undeclared copy of schema without review comment.
- **Only `internal/modules/<module>/postgres/` is scanned.** Stray query in service file, in `db/seeds/data.sql`, or inside migration is not. Whole subtree of that directory is scanned though, so `postgres/queries/` subpackage not a way out.
- **Ownership is per table, so column coupling is invisible.** `dashboard` depend on `order_items.unit_price`. Rename that column and `dashboard` break at runtime, in query only admin run, with `go build` unable to see inside SQL string.
- **Nothing the database does on its own.** View, trigger or function crossing modules is not Go source, not scanned.

**What you would do:** treat check as ratchet against regression, not proof of correctness. If you start building table names dynamically, replace grep with parser or ban the pattern — do not widen allowlist.

## The read-replica seam is built, configured, and unused

**Where you hit it:** you set `READER_DATABASE_URL`, expect read load to move, nothing change.

Everything except last link exist. `READER_DATABASE_URL` in `.env.example` and README's environment table. `database.NewReaderPostgres` build second pool. `transport/http/server.go` construct it at boot, store as `Deps.ReaderPool`. `database.ReadDB(ctx, primary, reader)` pick reader unless request marked by `database.WithRecentWrite`.

**No repository calls it.** Every adapter constructor is `New(pool
*pgxpool.Pool)` — one pool — and `Deps.ReaderPool` read by nothing outside `server.go`. Grep for `ReadDB` outside `internal/platform/database` and you find only that package's own tests. `WithRecentWrite` have no production caller either.

So setting variable open connection pool to replica that receive no queries. This the failure mode `ARCHITECTURE.md` warn about under multi-warehouse — knob that look wired and is not — in different place.

**What you would do:** either finish it or remove it. Finishing mean each `postgres` adapter taking both pools and routing genuinely read-only methods through `ReadDB`, plus deciding where `WithRecentWrite` applied (middleware after any non-GET, most likely) so read-your-own-write not hit lagging replica. Removing mean deleting config field, pool, `Deps` field, and two helpers. Leaving as-is mean next person to hit read-throughput problem will believe they already have replica.

## The charge job is dispatched but never enqueued

**Where you hit it:** you read `payment.Service.Process`, see it switch on `job.Action` with `case ActionCharge: return s.processChargeJob(...)`, and reasonably conclude charges run through job queue like refunds do. They do not. **No production code ever creates a `payment_jobs` row with
`action='charge'`** — all three `CreateJob` call sites in `payment/service.go` enqueue `ActionRefund`. So `processChargeJob` unreachable outside tests.

**Why it looks otherwise.** Charging happen inline instead, on two paths: `InitiatePayment` finalise synchronously when gateway capture funds immediately, and webhook finalise when it does not. Both call `FinalizePaymentSuccess` with **synthetic** `Job` carrying only `PaymentID`, `OrderID` and `Action` — no `ID`, because no row exist. Three consequences, individually invisible:

- `MarkJobCompleted(job.ID)` inside `FinalizePaymentSuccess` run `WHERE id = '00000000-0000-0000-0000-000000000000'` for those two callers. Deliberate no-op, not lost write — but `MarkJobCompleted` discard its rows-affected count, so nothing at runtime distinguish that from success.
- Webhook's follow-up `MarkJobCompletedByPaymentID(p.ID, ActionCharge)` also match zero rows, always.
- Test asserting "no pending charge job remains" pass whether or not bookkeeping run, because count zero either way. `test/e2e/fulfillment_failed_test.go` say so in comment instead of implying coverage it does not have.

**What you would do about it.** If charges should be queued — honest reading of `Process` — then `InitiatePayment` need to enqueue `ActionCharge` job and inline finalisation become worker's job, which also get you retry and backoff free on most failure-prone call in system. If they should not, delete `processChargeJob`, the `ActionCharge` case, and two `MarkJobCompleted*` calls that can only ever match nothing. Either is half-day. What cost more is current state, where reader must trace three call sites to discover queue they can see is not used.

Same shape as read-replica seam above: mechanism that exist, compile, is dispatched, and never run.

## The test suite shares one Postgres and one Redis, and slots are hand-assigned

**Where you hit it:** you add test package, copy a `TestMain` from neighbour, and another package's tests start failing intermittently.

`internal/testhelper` start two long-lived containers by fixed name — `go-api-test-postgres` and `go-api-test-redis` — and every test binary attach to whichever already exist. Isolation by **claimed slot**, not by container:

- **Postgres: one database per package.** `MustStartPostgres(dbName)` do `DROP DATABASE IF EXISTS <name> WITH (FORCE)` then `CREATE DATABASE`, then run migrations. Two packages passing same name will drop each other's database mid-run — `WITH (FORCE)` terminate other backend, so not even polite failure.
- **Redis: a hand-assigned integer.** `MustStartRedis(dbIndex)` take index from comment block in `internal/testhelper/testhelper.go`. Indices 0, 1, 2, 3, 5, and 6 claimed today (`platform/cache`, `transport/http/middleware`, `modules/user/postgres`, `transport/http`, `test/e2e`, `modules/user/redis`); 4 sit free. `ResetRedis` call `FlushDB`, so reusing index flush other package's fixtures.

Nothing enforce either claim. Duplicate name or index compile, pass review, and fail as flake in unrelated package — worst possible signal, because failure nowhere near the change.

Two further consequences worth knowing before writing tests:

- **`t.Parallel()` does not buy anything within a package**, because subtests share one database. Parallelism come from `go test` running package binaries concurrently — exactly why integration tests stay colocated instead of collapsing into one `test/integration` package (decision 11).
- **`make test` cannot run without Docker.** No build tags, no short mode. Every package touching Postgres or Redis fail outright.

**What you would do:** when adding test package, claim database name and (if it need Redis) next free index, and update registry comment in `internal/testhelper/testhelper.go` in same commit. If suite grow much past 15 Redis-using packages, index space run out and allocation must become dynamic.

## The composition site is deliberately tedious

**Where you hit it:** you open `internal/bootstrap/app.go` and find 15 aliased adapter imports (`cartpg`, `orderpg`, `userredis`, …) above a 75-line `New`.

Phase 0 made `bootstrap.New` the single composition root, so the tedium moved. It used to sit entirely in `internal/transport/http/router.go`; now `router.go` still carries its own 15 aliased imports — `authhttp`, `carthttp`, … — but every one of them is an `http` adapter (plus the dev-only mock gateway's route registrar), because `NewRouter` only ever builds routes from an already-wired `*bootstrap.App`. The `postgres`/`redis` half of the old pile — the half that actually constructs repositories — lives in `app.go` now, where every service gets built.

13 packages named `postgres`, 14 feature packages named `http`, and one named `redis`, so every site wiring them needs an alias. `cmd/worker/main.go` needs 3 of its own, for the two job-queue repositories and the payment worker's processor. `ARCHITECTURE.md` §0 and §3 own this: in a product codebase the subpackage split would be hard to justify, here it is the point — physical boundary makes `payment` importing `payment/postgres` a compile error, not a convention.

**What it costs beyond ugliness:** adding a feature means touching one long function every other feature also touches, so feature branches collide there. Neither `New` nor `NewRouter` carries a `//nolint:funlen` any more — both fit comfortably under the linter's 120-line limit (75 and 82 lines respectively) — so the tedium here is pure import-alias noise, not the cognitive-complexity kind `order.Service` and `payment.Service` still carry `//nolint` for elsewhere in the tree. Cost concentrated in one file on purpose, but still concentrated in _one_ file — now `app.go` instead of `router.go`.

**What you would do:** leave it. Splitting `New` per feature scatters the wiring graph, and a single readable list of every service and what it depends on is worth more than diff conflicts. If it becomes unbearable, split by _layer_ (build every repository, then every service) not by feature.

## A duplicate product id in a stock-adjustment map is silently dropped, not summed

**Where you hit it:** you build a `map[uuid.UUID]int` for `inventory.Service.ReserveBatch` / `DeductBatch` / `Restore` and write to the same product id twice while assembling it — `m[id] = 2`, then later `m[id] = 3` for the same `id`. No error, no sum. The map ends up holding `3`; the `2` is gone.

Every batch method on `inventory.Repository` and `inventory.Service` — `ReserveBatch`, `ReleaseBatch`, `DeductBatch`, `RestockBatch`, `Restore` — takes `map[uuid.UUID]int`, and `buildStockValues` (`internal/modules/inventory/postgres/repository.go`) builds its `VALUES` list straight off that map. A map holds one value per key, full stop, so there is nothing left to sum by the time this code runs. That is not the gap. The gap sits upstream of it: nothing stops the *construction* of the map from writing the same key twice, and when that happens the second write overwrites the first with no signal at all — not a panic, not an error return, not a log line.

**Why it is safe today.** Every current caller builds the map from data that cannot contain a duplicate product id before this code ever runs:

- `cart_items` carries `UNIQUE (cart_id, product_id)`, and `cart.Service.AddItem` upserts via `ON CONFLICT (cart_id, product_id) DO UPDATE` — a cart cannot hold two rows for one product.
- `order.Service.PlaceOrder` builds its reservation map one entry per cart-snapshot item, inheriting that guarantee directly — the map and the cart are keyed by the same product ids, one-for-one.
- `order_items` — read back by `finalizeFreeOrder`, `cancelWithReversal`, `releaseOrderHolds`, and payment's refund and charge-success paths via `ListItemsByOrderID` — has **no unique constraint on `(order_id, product_id)`**, only `PRIMARY KEY (id)` and a plain index on `order_id` (`db/migrations/20260424120005_create_orders.sql`). It is unique-per-product today only because the one write path that populates it (`PlaceOrder` → `repo.CreateItems`, one row per cart-snapshot item) already can't produce duplicates. The invariant holds one level removed from any enforcement of its own.

**What it costs.** Nothing in `inventory`, nothing in the four call sites, and no database constraint enforces this. "No current caller can trigger it" is a fact about the callers, not about the map type — the type permits the duplicate write; it just resolves it wrong, silently, when one happens.

**What breaks it.** Any future path that inserts an `order_items` row without going through the cart snapshot — a bulk admin order-creation endpoint, an import job, a split-shipment line-item model (see multi-warehouse, above) — could write two rows for the same product on one order without violating any constraint that exists today. The next map built from that data drops one of the two quantities silently; inventory reserves or deducts less than the order actually needs. Nothing crashes, nothing logs. The order just ships short.

**What you would do:** put `UNIQUE (order_id, product_id)` on `order_items` — the better fix, since an order-line model without split-shipment support has no legitimate reason for two rows on one product, and a constraint stops the bad row from existing rather than checking for it after the fact every time it's read. A `len(map) == len(items)` assertion at each of the four call sites is the cheaper stopgap if you want it sooner than a migration — but do not add that guard speculatively ahead of a real caller that needs it. Today none does; a guard for a case nothing can reach is exactly the kind of code this refactor was removing.

## `order/contract.Order` is one struct serving two different read shapes, and nothing marks which fields a given call path actually populates

**Where you hit it:** `order/contract.Order` has six fields (`ID`, `UserID`, `Total`, `Status`, `CouponCode`, `StockDeducted`, `StockReversed`, `Dispatched`). `order.Service.GetSnapshot` — payment's read — populates every field except `ID`/`UserID`. `order.Service.GetInfo` — shipping's ownership check — populates only `ID`, `UserID` and `Status`; the other five stay at their zero values. Both methods return the same Go type, so the compiler enforces nothing about which subset a given caller is allowed to read. A future change to `shipping.Service` that read `orderInfo.Dispatched` instead of comparing `Status` directly would compile clean and always observe `false` — not because the order is never dispatched, but because `GetInfo` never sets that field. Same story for `payment` if it ever received a value built by `GetInfo` instead of `GetSnapshot` — `Total` would read as a zero `money.Money{}` with an empty `Currency`, and `CouponCode` as `""` indistinguishable from "no coupon."

A two-types shape — `Snapshot` for payment, `Info` for shipping — would catch this at compile time: reading `.Total` off an `Info` value would not compile, because `Info` would have no such field. `contract.Order` ships as one type instead; the doc comments on it and on each of `GetSnapshot`/`GetInfo` say which fields a given path fills, but a doc comment is not something `go vet` or `golangci-lint` reads.

**What it costs.** Nothing today: `payment` only ever calls `GetSnapshot` and reads `Total`/`Status`/`CouponCode`/`StockDeducted`/`StockReversed`/`Dispatched`; `shipping` only ever calls `GetInfo` and reads `ID`/`UserID`/`Status`.

**What breaks it.** Any new consumer of either method, or any consumer that starts calling the other one instead of the one it was written against, silently reads zero values for fields the call path it's actually using never populates. No panic, no error, no lint warning — just a `Dispatched` that is always `false`, or a `Total` that is always zero, wherever someone reaches for the field the wrong method leaves unset.

**What you would do:** split `contract.Order` back into two types — `contract.Snapshot` (payment's six fields) and `contract.Info` (`ID`, `UserID`, `Status`) — so a call site that reads a field its method never populates fails to compile instead of running with a quietly wrong zero value. The two types would duplicate `Status` and nothing else, which is a small enough overlap that the duplication is cheaper than the blind spot. Do this the next time either port's shape changes, rather than as a standalone migration — `payment` and `shipping` would both need their port signatures and every call site touched either way.

## `cmd/worker` builds its own second handle on the tables `bootstrap.App`'s services already wrap

**Where you hit it:** `bootstrap.App` composes services (`Orders *order.Service`, `Payments *payment.Service`, …) and nothing storage-shaped beyond `TxRunner` and `Gateway`. `internal/platform/jobs.Runner[T]` needs two things per queue: a `Processor[T]` (`Process`, satisfied by the service) and a `Queue[T]` (`Claim` + `Prune`, satisfied only by the *repository* — `payment.Repository`, `notification.Repository`). A service never embeds or exposes its own repository (rule 9: a service holds no pool), so `App` has no field that is a `jobs.Queue[T]`. `cmd/worker/main.go` therefore calls `paymentpg.New(pool)` and `notificationpg.New(pool)` directly, right next to `bootstrap.New(bootstrap.Deps{Pool: pool, ...})`, which constructs its *own*, separate `paymentpg.New(d.Pool)` and `notificationpg.New(d.Pool)` internally to hand to `payment.NewService` / `notification.NewService`. Two independent repository values wrap the one pool for the same tables, built at two call sites that have no reference to each other. The comment beside the worker's copy says as much: "the queue side still needs the bare repository: a service holds no pool (rule 9), but Runner drains payment_jobs/notifications by Claim/Prune on the repository itself, not through the service."

**Why it is safe today.** `postgres.New(pool)` constructors here are stateless wrappers holding only the `*pgxpool.Pool` reference — no per-instance state, no cache, nothing that a second instance could get out of sync with the first. Two `paymentpg.New(pool)` values behave identically because they are, structurally, the same value twice.

**What it costs.** Nothing today. The cost is that the composition root is no longer the *only* place `cmd/worker` reaches for a feature's adapter — `internal/bootstrap` remains the sole importer of `paymentpg`/`notificationpg` inside `internal/`, satisfying rule 3, but `cmd/worker/main.go` (outside `internal/`, so outside `check-boundaries.sh`'s reach entirely) also imports them directly, for the queue-side handle `App` cannot provide.

**What breaks it.** The day a feature's `postgres.New` stops being a stateless pool wrapper — a connection-scoped cache, a per-instance prepared-statement handle, anything with state — the worker's standalone repository and `bootstrap.New`'s internal one stop being interchangeable, and nothing signals that the two construction sites need to agree. A reviewer checking "does the worker use the same wiring as the API" would have to know to look in two files instead of one.

**What you would do:** give `App` a way to hand back the `jobs.Queue[T]` alongside the `jobs.Processor[T]` for each queue-draining feature — either export the repository too (`App.PaymentRepo payment.Repository`, mirroring `TxRunner`/`Gateway`'s precedent of exposing infrastructure a binary needs beyond services), or add a constructor on `App` per queue (`func (a *App) PaymentQueue() payment.Repository`). Either removes `cmd/worker`'s standalone `paymentpg.New`/`notificationpg.New` calls and makes `bootstrap.New` the actual single source of every handle the worker binary needs, not just its services.

## Config load order is load-bearing and unchecked

**Where you hit it:** `order.LoadConfig(jobsLease time.Duration)` and `payment.LoadConfig(appEnv string, jobsLease time.Duration)` each validate their own timeout against a `jobsLease` parameter, not against `infra.Worker.LeaseDuration` directly. Both real call sites — `server.go`'s `loadModuleConfigs` helper and `cmd/worker/main.go`'s `run` — pass `infra.Worker.LeaseDuration` after `config.LoadInfra()` has already succeeded, and separately set `jobCfg.LeaseDuration = infra.Worker.LeaseDuration` for the job runner itself. Today those two reads of `infra.Worker.LeaseDuration` — one that gets validated, one that actually runs — agree only because both call sites read the same field of the same `infra` value in the same function. Nothing in either `LoadConfig`'s signature ties the parameter it validates to the value the runner will actually use: pass a different `time.Duration` — a leftover local, a value computed before infra finished loading, another config's default — and `LoadConfig` validates that number while the runner keeps using whatever `infra.Worker.LeaseDuration` actually resolved to. The two can diverge with neither `LoadConfig` nor the runner ever comparing them.

**Why it is safe today.** Both real call sites thread the same `infra.Worker.LeaseDuration` value through unchanged — from `LoadInfra`, to both `LoadConfig`s, to `jobCfg`. Every test in `internal/modules/order/config_test.go` / `payment/config_test.go` passes an explicit literal duration and checks the error, not the interaction with a separately-loaded infra value.

**What it costs.** Nothing today. Both call sites get it right, and a comment near each names why: `server.go`'s doc comment on `loadModuleConfigs` and `cmd/worker/main.go`'s inline comment above `auth.LoadConfig` both say "infra must load first" in roughly those words. A comment is not a compiler.

**What breaks it.** A future call site that passes a `time.Duration` other than `infra.Worker.LeaseDuration` — because it ran before infra finished loading, or reused a variable meant for something else — gets a validation result that says nothing about the lease the runner will actually use. If that placeholder value happens to land inside the range both `LoadConfig`s accept (above payment's 3×`PAYMENT_GATEWAY_TIMEOUT` floor, below order's `StaleProcessingThreshold` ceiling), boot succeeds having validated the wrong number, and the real `infra.Worker.LeaseDuration` — whatever it actually is, including a value outside that safe range — never gets checked against either threshold at all. That is how a worker ends up leasing jobs for longer than the recovery sweep waits before reverting them.

**What you would do:** nothing speculative ahead of a second call site that gets this wrong — today there are exactly two, and both thread the same value through by construction. If a third ever appears, either have `LoadInfra` return a lease-bearing type that both `LoadConfig`s require as their parameter instead of a bare `time.Duration` — a compile-time guarantee that the validated value came from a successful infra load — or keep the load sequence in the one function per binary that already owns it (`loadModuleConfigs`, `cmd/worker/main.go`'s `run`) and never inline a module's `LoadConfig` at a new use site.

## A contract package can grow into the shared domain model `internal/shared/` was rejected for being

**Where you hit it:** `<feature>/contract/` is imported by every consumer of that feature, so a field added there is public API. Nothing limits what may go in one, and the pressure is always to add "just one more field" rather than to ask why the consumer needs it. `db/OWNERSHIP.md`-style enforcement does not apply here: a struct field is not a table, so nothing machine-checks what a contract package is allowed to carry the way check 2 machine-checks table ownership.

`order/contract.Order` is already the shape this looks like from the inside: one struct with eight fields, populated differently by two different producer methods for two different consumers (see the entry above). That happened inside a single phase, with both call sites known in advance. A contract package accreting fields one unrelated PR at a time, each individually reasonable, is the same failure with no phase boundary forcing a review of the whole shape at once.

**What you would do:** before adding a field to a `contract/` package, check whether the consumer needs the *value* or the *decision*. `order/contract.Order.Dispatched bool` is a decision `order` already made; a `Status string` field plus the consumer re-deriving "is this dispatched" from it would be the model leaking instead — the same distinction `ARCHITECTURE.md` §10 draws between `payment.OrderUpdater.MarkPaid` (an intent) and an ad-hoc from/to status list (the mechanics). `ARCHITECTURE.md` decision 6 rejected `internal/shared/` for the same reason a contract package earns scrutiny: an owned, single-purpose surface with one publisher and named consumers stays legible; an unowned one that answers "what if we just add a field" enough times becomes a second copy of the schema with none of decision 6's ownership discipline.

## Context log attributes are write-only

**Where you hit it:** you want `request_id` in an error response body, or need to forward it as a header on an outbound call.

`logger.WithAttrs` stores a `[]slog.Attr` under an unexported key and only `ContextHandler.Handle` reads it. There is no accessor, and `middleware.GetRequestID` was deleted once nothing needed it. Both uses above need the value itself, not a log record.

**A second, sharper limit: nothing checks the single-naming invariant.** An attribute named at two points on one code path is emitted twice, and slog does not deduplicate keys. `payment.Service.Process` names `job_id` for the whole worker path; `FinalizePaymentSuccess` and `runCompensatingRefund` are also reached from `InitiatePayment` and `HandleWebhook`, which pass a `Job` literal with no `ID` and so deliberately name nothing. Add a fourth caller that names `job_id` itself and the worker path start emitting the key twice, with no test or linter to catch it. The check is a grep of the callers, run by hand.

This is not hypothetical. Naming `user_id` at the auth edge immediately collided with `user.Service.invalidateStatusCache`, which logged its own `user_id` — the user being acted upon, not the caller. On an admin role change the record carried both, and a last-wins parser kept the admin's id while silently dropping the target's. The fix was to rename the inner one to `target_user_id`, because the two values answer different questions.

**What you would do:** for the accessor, re-add a typed one beside the middleware that produces the value — `middleware.GetRequestID` as it was, a `context.WithValue` next to the `logger.WithAttrs` call. Do not add a generic `logger.Attrs(ctx)` reader: callers would loop the slice matching on a string key, which worse than the typed accessor it replaced. For the naming invariant, grep the tree for a key before naming it at a new edge — a collision look like nothing at all until someone query the logs.

---

## When not to copy this

Structure above priced for one situation: system with several genuinely distinct domains, expected to live long enough that boundaries pay back, worked on by more than one person or agent. Outside that, overhead.

Do not copy this if:

- **You have one domain.** Fourteen modules for one bounded context give you aliased imports, ports, mapper functions and ownership check with nothing on other side of ledger. Single package with `handler`/`service`/`repository` split is right shape.
- **You need cross-module queries more often than not.** Every join across boundary become two queries and a port, and reporting is the _only_ carve-out. If most reads are aggregates over many entities, you want schema you can join, or read model from start — not dozen ownership fences and one exception.
- **You are prototyping.** Rules cost most at beginning, when boundaries still guesses. Ports declared around domain you not understand yet are wrong ports, and harder to move than functions.
- **You want to extract services soon.** This structure make _code_ side of extraction cheap and leave data side entirely undone — 18 cross-module foreign keys in one schema. If service split imminent, separate data first; module boundaries will follow more easily than reverse.
- **You need multi-currency or multi-warehouse on day one.** Both named above as redesigns. Starting from design that rejected them mean undoing ratified decisions before writing first feature.

Do copy it if you want boundaries checkable not aspirational, and willing to pay two queries, a mapper and an import alias for that. That the whole trade, and every section above a line item in it.

---

Read alongside:

- `ARCHITECTURE.md` — the fourteen decisions and fifteen rejections these are shadow of.
- `db/OWNERSHIP.md` — table-ownership map, foreign-key inventory, and full blind-spot list for `make check-boundaries`.
- `AGENTS.md` — working rules, and which of them machine-checked.
