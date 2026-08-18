# Limitations this architecture creates

`ARCHITECTURE.md` record sixteen decisions and fifteen things this codebase deliberately not do. Every one bought something and charged for it. This file the invoice.

Exist because this repo a **template**: structure is product, someone about to copy into real system. Doc listing only what design make easy teach nothing — design never in danger of blame there. Reader need list of moments where they hit wall, so they recognize wall as this design's, not own mistake.

Each section state limitation, moment you meet it, what you must do about it. Where limitation is decision's shadow, decision cited not restated — read together.

Last section, [When not to copy this](#when-not-to-copy-this), for someone still deciding.

---

## You cannot filter or sort a product listing by stock

**Where you hit it:** you try add `?in_stock=true` to `GET /api/products`.

`products` owned by `product`, `inventory_levels` owned by `inventory` (decision 6, decision 7), so listing query cannot join them. Port is `query.InventoryReader.GetAvailability(ctx, ids)` (`internal/modules/product/usecase/query/ports.go`) — batch-shaped, asked _after_ page already selected. `product/usecase/query.UseCase.ListPublished` calls `repo.ListPublished` then `enrich`, that order, cannot be other order: enrich need ids page chose.

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

**Where you hit it:** you read `product/usecase/query.UseCase.ListPublished` and count round trips.

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
updated_at`. **No price column**. `cart/usecase/query.UseCase.GetCart` read lines then call `products.GetInfoByIDs` for current name, price, status, available stock. So cart not display stale price — never pinned one at all. Every read current.

Note this cut against intuitive framing. Cart's total never _stale_; it **unpinned**. What genuinely point-in-time snapshot is `available_stock` each line report: true when cart read, can be wrong by time `PlaceOrder` run — why checkout re-reserve against `inventory` instead of trusting what cart displayed.

`order` do snapshot prices at placement, so an _order_ internally consistent forever. Cart not, and not meant to be.

**What you would do:** if need held price, add `unit_price` and `currency` to `cart_items`, write them at add-time, then decide the thing that actually make this hard — what happen when held price and current price disagree at checkout. Reprice silently? Refuse? Show both? Price column without answer to that question just move surprise later.

## An unsellable cart line is shown, not hidden

**Where you hit it:** product archived, unpublished or deleted after customer added it, and `GET /api/cart` still return line — with `"sellable": false`, excluded from `total`.

Behaviour change, and chosen. Previous implementation drop line with `JOIN … AND p.deleted_at IS NULL`, so customer's total fell with nothing on screen to explain. If product record gone entirely, `cart/usecase/query.UseCase.GetCart` substitute synthetic `&Product{Status: "unavailable"}` placeholder instead of dropping item.

**What it costs:** every client rendering cart must handle `sellable: false`, and client written against old behaviour will show line it cannot check out. `Cart.Total()` fold sellable lines only, so `total` will not equal sum of line prices client can see — look like bug if you not read this.

**What you would do:** render unsellable lines distinctly, offer remove action. Do not re-add the `JOIN` filter.

## A mixed-currency cart is a 400 from `GET /api/cart`

**Where you hit it:** cart contain items priced in different currencies, endpoint that used to return 200 now return 400.

`money.Money.Add` refuse to sum across currencies, so `cart.Cart.Total()` return `(money.Money, error)`, and `internal/modules/cart/usecase/query/http/handler.go`'s `Get` propagate that error instead of publishing total it could not compute. Error wrap `apperror.ErrBadRequest` alongside `money.ErrCurrencyMismatch`, because `ErrCurrencyMismatch` alone match no case in `response.HandleErr` and would surface as 500 for what is plainly user input.

**Nothing prevents such a cart existing.** Prices per-product and `cart.AddItem` not constrain them, so catalogue with mixed currencies will produce this. Checkout already reject it; this change make `GET /api/cart` agree with `PlaceOrder` instead of showing number denominated in nothing.

Two things soften it, both deliberate: `Total()` fold **sellable lines only**, so archived line in foreign currency still yield clean 200; and empty cart yield `total: 0` not error.

**What you would do:** decide currency at level above product. Constrain catalogue to one currency, or scope carts by currency and reject the add not the read. The 400 is correct report of inconsistent cart; not the place to fix it.

## `promotion` and `dashboard` amounts are plain `int64`

**Where you hit it:** you reach for `money.Money` in `promotion` or `dashboard` and find nothing to construct it from. Neither package reference `money` at all.

`money.Money` cover exactly four features — `order`, `payment`, `product`, `cart`. `ARCHITECTURE.md` §10 state why other two are out, and reasons load-bearing not bookkeeping:

- `Promotion.Value int64` is **polymorphic**. With `Type == TypePercentage` it a percentage; with `TypeFixedAmount` it minor units. `money.New(10,
"USD")` to mean "10%" would be value object asserting something false. `promotion` also have no currency field anywhere, so its genuinely monetary `MinOrderAmount`, `MaxDiscount` and `CouponUsage.Discount` have nothing to pair with.
- `dashboard` aggregate revenue across all orders and have no currency field, so any currency it named would be guess.

**What it costs:** seam between `order` and `promotion` untyped in middle. `place.CouponReserver.Reserve` (`internal/modules/order/usecase/place/ports.go`) pass `orderSubtotal int64` and return `discountAmount int64`, and `order/usecase/place/usecase.go` re-pair it with `money.New(discount, subtotal.Currency)` on own side. That also where clamp live — `max(subtotal-discount, 0)` as plain arithmetic, because `Money.Sub` deliberately not decide whether money may go negative. If you add multi-currency promotion system, this seam where type safety missing and where wrong-currency discount would pass unnoticed.

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

Seven checks, all greps, none a compiler. `db/OWNERSHIP.md` documents check 3's (table ownership) gaps in full; the ones most likely to bite:

- **Table names must be literals.** Check 3 greps for the identifier after `FROM` / `JOIN` / `INSERT INTO` / `UPDATE` / `TRUNCATE` / `COPY`. Every query today has its table name in a string literal, but `fmt.Sprintf` is already routine in these adapters for `WHERE` fragments and placeholder lists. The habit of assembling SQL exists; it simply has not reached a table name yet. The day it does, the check goes quiet instead of failing. `pgx.CopyFrom` would be the same hole immediately: its table is a `pgx.Identifier` Go value with no keyword in front. Nothing uses it today.
- **Prose in a production string literal is a false positive.** Comments and `_test.go` files excluded, but `"update orders failed"` in a slice's `postgres/` package still reports `orders`. Fails loudly, not quietly — the right direction, but the failure mode that gets a check disabled.
- **Test files are skipped, deliberately.** A test seeds sibling tables to satisfy foreign keys — fixture setup, not an architectural crossing. Cost: a real violation living in a `_test.go` helper is invisible.
- **`dashboard` is exempt wholesale.** Nothing verifies it stays read-only — no grant, no separate role, no check. An `UPDATE` in `internal/modules/dashboard/usecase/*/postgres` passes CI today. Nothing bounds it to today's two tables either, so "may read anything" can become a second, undeclared copy of the schema without a review comment.
- **Ownership is per table, so column coupling is invisible.** `dashboard` depends on `order_items.unit_price`. Rename that column and `dashboard` breaks at runtime, in a query only an admin runs, with `go build` unable to see inside the SQL string.
- **Nothing the database does on its own.** A view, trigger or function crossing modules is not Go source, not scanned.

Two blind spots outside check 3, worth naming because they are new to this phase — checks 1 and 3 both ran on effectively nothing for the length of the migration, and check 4's contract-only rule is still a regex over a quoted import path — each gets its own entry below rather than a bullet here, since each has its own "where you hit it."

**What you would do:** treat check as ratchet against regression, not proof of correctness. If you start building table names dynamically, replace grep with parser or ban the pattern — do not widen allowlist.

## The boundary checks ran on nothing for the length of the migration

**Where you hit it:** you bisect a boundary violation and find it was introduced during phase 2, in a commit where `make check-boundaries` passed.

Checks 1 and 3 are path-based: check 1 exempts `internal/modules/<feature>/usecase/<slice>/http/`, and check 3 (before this phase) scanned only a `postgres/` directory. From the moment a module was sliced until this phase rewrote both, every path either check keyed off had already moved deeper than the check expected, so both matched nothing in that module and reported nothing — verified during this phase by planting a `json` tag on a domain type and a foreign `FROM orders` in a command, in an already-sliced module, both of which passed a green `check-boundaries` before the fix.

The same trap was set again by the move to `usecase/`, and this time it was answered rather than rediscovered: `scripts/boundaries_test.go` now plants a probe file in a real slice — a `json` tag, a sibling-slice import, an `internal/transport` import — and asserts `check-boundaries.sh` reports each one. A path-keyed check that has quietly stopped matching anything fails that test instead of printing `Boundaries OK`.

**What you would do:** on the next structural migration, rewrite the checks against a hand-built pilot module first and run both shapes — old check, new tree — in parallel, rather than choosing to rewrite the checks at the end. The cost of the choice made in the earlier phase is that the first green run of the real checks came after every module had already moved, which is exactly backwards from when a boundary check is most useful. The probe test is the ratchet that stops it being paid a third time; it is not a substitute for doing the checks first.

## A slice can still smuggle SQL past check 3 with a built string

**Where you hit it:** you write `db.Query(ctx, "SELECT * FROM "+table)` or hand a variable to `pgx.CopyFrom`, and `make check-boundaries` says nothing.

Check 3 now scans every non-test `.go` file under a module, so SQL in a slice's `usecase.go` is caught exactly like SQL in its `postgres/` adapter — the hole the previous entry describes is closed. It still only matches string literals. A table name assembled with `+` or `fmt.Sprintf`, or reaching `pgx.CopyFrom` as a `pgx.Identifier` value, is invisible either way; widening which directories get scanned changes nothing about what the pattern itself can see.

**What you would do:** nothing cheap. A real fix parses Go. The literal-only scan catches every violation anyone has written by accident, and none written on purpose — which is the trade every grep-based check in this script makes, not a gap unique to this one.

## The read-replica seam is built, configured, and unused

**Where you hit it:** you set `READER_DATABASE_URL`, expect read load to move, nothing change.

Everything except last link exist. `READER_DATABASE_URL` in `.env.example` and README's environment table. `database.NewReaderPostgres` build second pool. `transport/http/server.go` construct it at boot, store as `Deps.ReaderPool`. `database.ReadDB(ctx, primary, reader)` pick reader unless request marked by `database.WithRecentWrite`.

**No repository calls it.** Every adapter constructor is `New(pool
*pgxpool.Pool)` — one pool — and `Deps.ReaderPool` read by nothing outside `server.go`. Grep for `ReadDB` outside `internal/platform/database` and you find only that package's own tests. `WithRecentWrite` have no production caller either.

So setting variable open connection pool to replica that receive no queries. This the failure mode `ARCHITECTURE.md` warn about under multi-warehouse — knob that look wired and is not — in different place.

**What you would do:** either finish it or remove it. Finishing mean each `postgres` adapter taking both pools and routing genuinely read-only methods through `ReadDB`, plus deciding where `WithRecentWrite` applied (middleware after any non-GET, most likely) so read-your-own-write not hit lagging replica. Removing mean deleting config field, pool, `Deps` field, and two helpers. Leaving as-is mean next person to hit read-throughput problem will believe they already have replica.

## The charge job is dispatched but never enqueued

**Where you hit it:** you read `payment/jobs.Dispatcher.Process`, see it switch on `job.Action` with `case domain.ActionCharge: return d.charge.ProcessCharge(ctx, job)`, and reasonably conclude charges run through the job queue like refunds do. They do not. **No production code ever creates a `payment_jobs` row with
`action='charge'`** — all three call sites that create a job (two in `payment/usecase/charge/usecase.go`, one in `payment/usecase/refund/usecase.go`) go through `jobs.Queue.EnqueueRefund`, which hardcodes `domain.ActionRefund`. So `charge.UseCase.ProcessCharge` is unreachable outside tests, even though the dispatcher routes to it correctly.

**Why it looks otherwise.** Charging happens inline instead, on two paths: `charge.UseCase.InitiatePayment` finalises synchronously when the gateway captures funds immediately, and `webhook.UseCase`'s callback finalises when it does not. Both call `charge.UseCase.FinalizePaymentSuccess` with a **synthetic** `Job` carrying only `PaymentID`, `OrderID` and `Action` — no `ID`, because no row exists. Three consequences, individually invisible:

- `MarkJobCompleted(job.ID)` inside `FinalizePaymentSuccess` runs `WHERE id = '00000000-0000-0000-0000-000000000000'` for those two callers. Deliberate no-op, not a lost write — but `MarkJobCompleted` discards its rows-affected count, so nothing at runtime distinguishes that from success.
- The webhook's follow-up `MarkJobCompletedByPaymentID(p.ID, ActionCharge)` also matches zero rows, always.
- A test asserting "no pending charge job remains" passes whether or not the bookkeeping ran, because the count is zero either way. `test/e2e/fulfillment_failed_test.go` says so in a comment instead of implying coverage it does not have.

**What you would do about it.** If charges should be queued — the honest reading of `Dispatcher.Process` — then `charge.UseCase.InitiatePayment` needs to enqueue an `ActionCharge` job and inline finalisation becomes the worker's job, which also gets you retry and backoff free on the most failure-prone call in the system. If they should not, delete `ProcessCharge`, the `ActionCharge` case in `Dispatcher.Process`, and the two `MarkJobCompleted*` calls that can only ever match nothing. Either is half a day. What costs more is the current state, where the reader must trace three call sites across two slices to discover a queue path they can see is not used.

Same shape as read-replica seam above: mechanism that exist, compile, is dispatched, and never run.

## The test suite shares one Postgres and one Redis, and slots are hand-assigned

**Where you hit it:** you add test package, copy a `TestMain` from neighbour, and another package's tests start failing intermittently.

`internal/testhelper` start two long-lived containers by fixed name — `go-api-test-postgres` and `go-api-test-redis` — and every test binary attach to whichever already exist. Isolation by **claimed slot**, not by container:

- **Postgres: one database per module now, not per package.** Since phase 1, `MustStartPostgres(dbName)` create and migrate `dbName` once, under an advisory lock, and nothing ever drop it — every slice's test package under one module passes the same name on purpose, for all fourteen modules now (`"test_shipping"`, `"test_order"`, `"test_payment"`, … — `grep -rn 'MustStartPostgres(' --include='*_test.go' internal/modules` shows the current mapping). See [Slice test packages share a database and never get a clean table](#slice-test-packages-share-a-database-and-never-get-a-clean-table) for the cost that trades in for.
- **Redis: a hand-assigned integer, tracked by a comment nothing checks.** `MustStartRedis(dbIndex)` take an index the caller picks against the registry comment above that function in `internal/testhelper/testhelper.go`: 0, 1, 3, 5 and 6 claimed (`platform/cache`, `transport/http/middleware`, `transport/http`, `test/e2e`, `modules/user/usecase/query/redis`); 2 and 4 free. The comment is prose — it drifts the moment someone forgets it, and `grep -rn 'MustStartRedis(' --include='*_test.go' .` is the only record that cannot. `ResetRedis` call `FlushDB`, so reusing index flush other package's fixtures.

Nothing enforce either claim, and losing the comment removed the one place a reader would have looked before guessing. Duplicate name or index compile, pass review, and fail as flake in unrelated package — worst possible signal, because failure nowhere near the change.

Two further consequences worth knowing before writing tests:

- **`t.Parallel()` does not buy anything within a package**, because subtests share one database. Parallelism come from `go test` running package binaries concurrently — exactly why integration tests stay colocated instead of collapsing into one `test/integration` package (decision 11).
- **`make test` cannot run without Docker.** No build tags, no short mode. Every package touching Postgres or Redis fail outright.

**What you would do:** when adding test package, pass the owning module's existing database name, and (if it need Redis) grep the five call sites above for a free index before taking one. If suite grow much past 15 Redis-using packages, index space run out and allocation must become dynamic.

## Slice test packages share a database and never get a clean table

**Where you hit it:** you write a slice test asserting `SELECT count(*)`, and it pass alone and fail under `go test ./...`.

Every module is sliced now, so this is universal, not a `shipping`-only shape: `internal/modules/shipping/usecase/query/postgres`, `create/postgres`, `updatetracking/postgres` and `deliver/postgres` are four separate test binaries, and all four call `testhelper.MustStartPostgres("test_shipping")` — same name, on purpose. `order` does the same across nine test packages sharing `test_order`, `payment` across five sharing `test_payment`, and so on for the rest (`grep -c` the mapping above rather than trust a number here — it moves with every test file added). Since phase 1, that function create and migrate a database once, under an advisory lock, and never drop it: dropping it mid-run would tear down whichever sibling package still hold it open. So there is no `ResetDB` between any of a module's own slice packages and no clean table to assume, even though `go test ./...` runs them concurrently against the one database.

**What you would do:** seed the rows your subtest owns and scope every assertion to them — by a freshly generated `uuid.New()`, the way shipping's own tests do (`seedOrder`, `seedShipment`). Never `TRUNCATE`: the sibling package whose rows you delete is not in your file, and will fail somewhere else, possibly minutes later in an unrelated CI run. Nothing enforce this — the failure look like a flake in a package you never touched.

## Two things the test suite cannot see: which route group a slice lands on, and where its write sits inside its own transaction

**Where you hit it, the first way:** you move a route to the wrong `middleware.RouteGroup` in `internal/transport/http/routes/<feature>.go` — admin instead of authed, or the reverse — and every test still passes.

A slice's own `handler_test.go` builds its own `middleware.NewRouteGroup` and writes the prefix itself, rather than importing anything from the real router. 55 of the 56 test files in slice `http/` packages do this; the exception, `order/usecase/query/http/admin_handler_test.go`, calls the handler directly instead. It is not even the same prefix: `internal/modules/category/usecase/create/http/handler_test.go:175` builds `middleware.NewRouteGroup(mux, "/api/v1/admin")`, while `internal/transport/http/router.go` mounts the real admin group at `/api/admin` — no `/v1` anywhere in production. The test passes either way, because it never touches the real router, only its own hand-built stand-in.

`internal/transport/http/router_test.go` and `test/e2e` are the only things that drive the real `NewRouter` and would catch a route landing on the wrong group — and **neither snapshots the whole table**. `router_test.go` asserts a sample — 24 distinct `/api…` paths appear in the whole file (`grep -oE '"/api[^"]*"' internal/transport/http/router_test.go | sort -u | wc -l`), against 64 routes in the table, and several of those 24 only assert a 401 or a 403 rather than the handler behind them. The rest are covered only where an e2e saga happens to exercise that exact route.

**Decision 15 changed where this mistake gets made, not whether it can be made.** The URL now lives one tree over from the handler, in `internal/transport/http/routes/`, so **adding a slice with a route touches two trees** and the wrong-group edit is made in the route file rather than in the module. What that buys is that every URL in the system is in one directory, 14 files, so a route-table snapshot test is now a thing someone could actually write in an afternoon — under the old shape it would have had to read 14 modules. What it costs is that nothing links the two halves: a handler with no route file mounting it compiles, lints, passes `check-boundaries`, and serves nothing, and no check anywhere says so.

**Where you hit it, the second way:** a sliced command's repository write moves outside its own `tx.Run` callback — a bug that should fail a test — and nothing fails.

`testhelper.FakeTxRunner.Run` (`internal/testhelper/txrunner.go`) is `return fn(ctx)`: it calls the callback inline, with no transaction underneath it, so a mock-based `usecase_test.go` cannot observe whether a repository call happened inside `tx.Run`'s closure or leaked outside it — both look identical to the fake. Nine slices are in that position today (`grep -rl FakeTxRunner --include='*_test.go' internal/modules`): `cart/usecase/add`, `order/usecase/{cancel,expire,place}`, `payment/usecase/{charge,refund}`, `promotion/usecase/reserve`, `shipping/usecase/{create,deliver}`.

**What you would do:** for the first, either assert the mounted prefix somewhere real-router-shaped — a route-table snapshot test driven through `apihttp.NewRouter` itself, which decision 15 made cheap by putting all 64 routes in one directory — or accept that `router_test.go` plus `test/e2e` is the only backstop and say so out loud, which is what this entry does. For the second, a real `TxRunner` backed by a test transaction (not the fake) would let a test assert call order across the boundary, at the cost of every affected `usecase_test.go` needing a real Postgres connection instead of a mock — trading a fast unit test for a slower, more honest one. Neither fix is free; neither is done here.

## The composition site is deliberately tedious

**Where you hit it:** you open `internal/bootstrap/app.go` expecting the pile of adapter aliases a template this size usually carries, and find six.

Phase 0 made `bootstrap.New` the single composition root, so the tedium moved there first — and then phase 2's slicing mostly moved it back out again, one level down. `app.go` no longer imports a module's `postgres`/`http`/`redis` adapter at all: it imports each module by its unaliased root package (`auth`, `cart`, `category`, …) and calls that module's own `New`, which wires its own slices' adapters inside its own `module.go`. The six aliases that remain — `ordercancel`, `ordercancelpg`, `ordertransition`, `ordertransitionpg`, `orderquery`, `orderquerypg` — exist only because breaking the order/payment cycle needs two pieces of `order` (`usecase/transition`, `usecase/query`) built before `order.New` itself can run, so `app.go` reaches two levels past `order`'s module boundary for those two alone. `func New` is 63 lines (`internal/bootstrap/app.go:61`-`123`).

`internal/transport/http/router.go` used to keep its own pile — 14 aliased `http` imports, one per feature, plus the dev-only mock gateway's registrar. Decision 15 emptied it: `router.go` now holds **one** aliased import (`mockgatewayserver`), imports `routes` unaliased, and calls fourteen functions — `routes.Auth(...)`, `routes.Cart(...)` — inside a 62-line `NewRouter`. The aliases did not vanish, they moved: `internal/transport/http/routes/<feature>.go` imports that feature's slice handlers, 3 to 5 aliases per file, 14 files. That is one more file per feature than the old shape, and the honest description is that the pile was redistributed rather than paid off. What it bought is that every URL in the system is now readable in one directory. `cmd/worker/main.go` needs one alias of its own (`paymentworker`, for the processor that wraps payment's dispatcher plus order's housekeeping sweep) — down from three, now that it reads the queue and processor straight off `bootstrap.App` instead of building its own second `paymentpg`/`notificationpg` repository handles.

64 packages named `postgres` and 53 named `http` exist under `internal/modules` today, one named `redis` — re-run `find internal/modules -type d -name postgres | wc -l` (and `http`, `redis`) rather than trust these; they move with every slice added. The `http` figure was 67 before decision 15 took the 14 feature route tables out of the modules. Every one of them still needs an alias somewhere, just mostly inside the `module.go` or the `routes/<feature>.go` that wires it rather than at the top of the binary. `ARCHITECTURE.md` §0 and §3 own this: in a product codebase the subpackage split would be hard to justify, here it is the point — physical boundary makes a slice importing its own `postgres/` a compile error, not a convention.

**What it costs beyond ugliness:** adding a feature means touching `app.go` once (one line to build the module, one field on `App`), `router.go` once (one `routes.<Feature>(...)` call), and creating `routes/<feature>.go` — three files every feature collides on, up from two. Adding a *route* to an existing feature touches two trees: the slice for the handler, `routes/<feature>.go` for the URL. Neither `New` nor `NewRouter` carries a `//nolint:funlen`; both fit comfortably under the linter's 120-line limit, so the tedium here is pure import-alias noise, not the cognitive-complexity kind five slices' `usecase.go` files still carry `//nolint` for elsewhere in the tree (see AGENTS.md's Guardrails section for the current list). Cost concentrated in one file per binary-wide concern, deliberately.

**What you would do:** leave it. Splitting `New` per feature scatters the wiring graph, and a single readable list of every module and what it depends on is worth more than diff conflicts. If it becomes unbearable, split by _layer_ (build every module's dependencies, then every module) not by feature.

## A duplicate product id in a stock-adjustment map is silently dropped, not summed

**Where you hit it:** you build a `map[uuid.UUID]int` for `inventory/usecase/reserve.UseCase.ReserveBatch` / `inventory/usecase/deduct.UseCase.DeductBatch` / `inventory/usecase/restore.UseCase.Restore` and write to the same product id twice while assembling it — `m[id] = 2`, then later `m[id] = 3` for the same `id`. No error, no sum. The map ends up holding `3`; the `2` is gone.

Every batch method — `ReserveBatch`, `ReleaseBatch`, `DeductBatch`, `RestockBatch`, `Restore` — takes `map[uuid.UUID]int`, now spread across three sibling slices (`reserve`, `deduct`, `restore`) each with its own `Repository` and its own `buildStockValues` (`internal/modules/inventory/usecase/{reserve,deduct,restore}/postgres/repository.go`) building its `VALUES` list straight off that map. A map holds one value per key, full stop, so there is nothing left to sum by the time this code runs. That is not the gap. The gap sits upstream of it: nothing stops the _construction_ of the map from writing the same key twice, and when that happens the second write overwrites the first with no signal at all — not a panic, not an error return, not a log line.

**Why it is safe today.** Every current caller builds the map from data that cannot contain a duplicate product id before this code ever runs:

- `cart_items` carries `UNIQUE (cart_id, product_id)`, and `cart/usecase/add.UseCase.Execute` upserts via `ON CONFLICT (cart_id, product_id) DO UPDATE` — a cart cannot hold two rows for one product.
- `order/usecase/place.UseCase.Execute` builds its reservation map one entry per cart-snapshot item, inheriting that guarantee directly — the map and the cart are keyed by the same product ids, one-for-one.
- `order_items` — read back by `place.UseCase.finalizeFreeOrder`, `cancel.UseCase.cancelWithReversal`, `expire.UseCase.releaseOrderHolds`, and payment's refund and charge-success paths via `order/usecase/query.UseCase.ListItemQuantities` (reached through the standalone `order/usecase/query.UseCase` value — `orderQuery` in `internal/bootstrap/app.go`, built before `order.New` can run so it can wire into `payment` and break the order/payment construction cycle — which is what `refund.OrderItemsGetter` and `charge.OrderItemsGetter` both name; payment never receives `order.Module` itself) — has **no unique constraint on `(order_id, product_id)`**, only `PRIMARY KEY (id)` and a plain index on `order_id` (`db/migrations/20260424120005_create_orders.sql`). It is unique-per-product today only because the one write path that populates it (`place.UseCase.Execute` → `repo.CreateItems`, one row per cart-snapshot item) already can't produce duplicates. The invariant holds one level removed from any enforcement of its own.

**What it costs.** Nothing in `inventory`, nothing in the four call sites, and no database constraint enforces this. "No current caller can trigger it" is a fact about the callers, not about the map type — the type permits the duplicate write; it just resolves it wrong, silently, when one happens.

**What breaks it.** Any future path that inserts an `order_items` row without going through the cart snapshot — a bulk admin order-creation endpoint, an import job, a split-shipment line-item model (see multi-warehouse, above) — could write two rows for the same product on one order without violating any constraint that exists today. The next map built from that data drops one of the two quantities silently; inventory reserves or deducts less than the order actually needs. Nothing crashes, nothing logs. The order just ships short.

**What you would do:** put `UNIQUE (order_id, product_id)` on `order_items` — the better fix, since an order-line model without split-shipment support has no legitimate reason for two rows on one product, and a constraint stops the bad row from existing rather than checking for it after the fact every time it's read. A `len(map) == len(items)` assertion at each of the four call sites is the cheaper stopgap if you want it sooner than a migration — but do not add that guard speculatively ahead of a real caller that needs it. Today none does; a guard for a case nothing can reach is exactly the kind of code this refactor was removing.

## `order/contract.Order` is one struct serving two different read shapes, and nothing marks which fields a given call path actually populates

**Where you hit it:** `order/contract.Order` has eight fields (`ID`, `UserID`, `Total`, `Status`, `CouponCode`, `StockDeducted`, `StockReversed`, `Dispatched`). `order/usecase/query.UseCase.GetSnapshot` — payment's read — populates every field except `ID`/`UserID`. `order/usecase/query.UseCase.GetInfo` — shipping's ownership check — populates only `ID`, `UserID` and `Status`; the other five stay at their zero values. Both methods return the same Go type, so the compiler enforces nothing about which subset a given caller is allowed to read. A future change to `shipping/usecase/create.UseCase.Execute` — which already reads `order.Status` via `domain.CanShipOrder` — that read `orderInfo.Dispatched` instead would compile clean and always observe `false` — not because the order is never dispatched, but because `GetInfo` never sets that field. Same story for `payment` if it ever received a value built by `GetInfo` instead of `GetSnapshot` — `Total` would read as a zero `money.Money{}` with an empty `Currency`, and `CouponCode` as `""` indistinguishable from "no coupon."

**Related, and still open today:** `payment/usecase/refund.UseCase`'s finalize step (`internal/modules/payment/usecase/refund/usecase.go:189` and `:192`) branches on `orderSnap.Dispatched` and `orderSnap.StockReversed` — exactly the two fields this entry is about — and neither branch has a test that sets either field `true`. That is a pre-existing gap, not something this phase introduced, and it means the type-safety hole above and the untested branch are the same two fields, found from two different directions.

A two-types shape — `Snapshot` for payment, `Info` for shipping — would catch this at compile time: reading `.Total` off an `Info` value would not compile, because `Info` would have no such field. `contract.Order` ships as one type instead; the doc comments on it and on each of `GetSnapshot`/`GetInfo` say which fields a given path fills, but a doc comment is not something `go vet` or `golangci-lint` reads.

**What it costs.** Nothing today: `payment` only ever calls `GetSnapshot` and reads `Total`/`Status`/`CouponCode`/`StockDeducted`/`StockReversed`/`Dispatched`; `shipping` only ever calls `GetInfo` and reads `ID`/`UserID`/`Status`.

**What breaks it.** Any new consumer of either method, or any consumer that starts calling the other one instead of the one it was written against, silently reads zero values for fields the call path it's actually using never populates. No panic, no error, no lint warning — just a `Dispatched` that is always `false`, or a `Total` that is always zero, wherever someone reaches for the field the wrong method leaves unset.

**What you would do:** split `contract.Order` back into two types — `contract.Snapshot` (payment's six fields) and `contract.Info` (`ID`, `UserID`, `Status`) — so a call site that reads a field its method never populates fails to compile instead of running with a quietly wrong zero value. The two types would duplicate `Status` and nothing else, which is a small enough overlap that the duplication is cheaper than the blind spot. Do this the next time either port's shape changes, rather than as a standalone migration — `payment` and `shipping` would both need their port signatures and every call site touched either way.

## Config load order is load-bearing and unchecked

**Where you hit it:** `order.LoadConfig(jobsLease time.Duration)` and `payment.LoadConfig(appEnv string, jobsLease time.Duration)` each validate their own timeout against a `jobsLease` parameter, not against `infra.Worker.LeaseDuration` directly. Both real call sites — `server.go`'s `loadModuleConfigs` helper and `cmd/worker/main.go`'s `run` — pass `infra.Worker.LeaseDuration` after `config.Load()` has already succeeded. Only `cmd/worker/main.go` goes on to drain a queue: it separately sets `jobCfg.LeaseDuration = infra.Worker.LeaseDuration` for the job runner itself (`server.go` builds no `jobs.Runner` and has no `jobCfg` — the API binary validates the lease but never runs one). In the worker, those two reads of `infra.Worker.LeaseDuration` — one that gets validated, one that actually runs — agree only because both are read from the same `infra` value in the same function. Nothing in either `LoadConfig`'s signature ties the parameter it validates to the value the runner will actually use: pass a different `time.Duration` — a leftover local, a value computed before infra finished loading, another config's default — and `LoadConfig` validates that number while the runner keeps using whatever `infra.Worker.LeaseDuration` actually resolved to. The two can diverge with neither `LoadConfig` nor the runner ever comparing them.

**Why it is safe today.** Both real call sites thread the same `infra.Worker.LeaseDuration` value through unchanged from `Load` to both `LoadConfig`s; `cmd/worker/main.go` threads it on into `jobCfg` as well, since it is the only one of the two that builds a runner. Every test in `internal/modules/order/config_test.go` / `payment/config_test.go` passes an explicit literal duration and checks the error, not the interaction with a separately-loaded infra value.

**What it costs.** Nothing today. Both call sites get it right, and a comment near each names why: `server.go`'s doc comment on `loadModuleConfigs` and `cmd/worker/main.go`'s inline comment above `auth.LoadConfig` both say "infra must load first" in roughly those words. A comment is not a compiler.

**What breaks it.** A future call site that passes a `time.Duration` other than `infra.Worker.LeaseDuration` — because it ran before infra finished loading, or reused a variable meant for something else — gets a validation result that says nothing about the lease the runner will actually use. If that placeholder value happens to land inside the range both `LoadConfig`s accept (above payment's 3×`PAYMENT_GATEWAY_TIMEOUT` floor, below order's `StaleProcessingThreshold` ceiling), boot succeeds having validated the wrong number, and the real `infra.Worker.LeaseDuration` — whatever it actually is, including a value outside that safe range — never gets checked against either threshold at all. That is how a worker ends up leasing jobs for longer than the recovery sweep waits before reverting them.

**What you would do:** nothing speculative ahead of a second call site that gets this wrong — today there are exactly two, and both thread the same value through by construction. If a third ever appears, either have `Load` return a lease-bearing type that both `LoadConfig`s require as their parameter instead of a bare `time.Duration` — a compile-time guarantee that the validated value came from a successful infra load — or keep the load sequence in the one function per binary that already owns it (`loadModuleConfigs`, `cmd/worker/main.go`'s `run`) and never inline a module's `LoadConfig` at a new use site.

## A contract package can grow into the shared domain model `internal/shared/` was rejected for being

**Where you hit it:** `<feature>/contract/` is imported by every consumer of that feature, so a field added there is public API. Nothing limits what may go in one, and the pressure is always to add "just one more field" rather than to ask why the consumer needs it. `db/OWNERSHIP.md`-style enforcement does not apply here: a struct field is not a table, so nothing machine-checks what a contract package is allowed to carry the way check 2 machine-checks table ownership.

`order/contract.Order` is already the shape this looks like from the inside: one struct with eight fields, populated differently by two different producer methods for two different consumers (see the entry above). That happened inside a single phase, with both call sites known in advance. A contract package accreting fields one unrelated PR at a time, each individually reasonable, is the same failure with no phase boundary forcing a review of the whole shape at once.

**What you would do:** before adding a field to a `contract/` package, check whether the consumer needs the _value_ or the _decision_. `order/contract.Order.Dispatched bool` is a decision `order` already made; a `Status string` field plus the consumer re-deriving "is this dispatched" from it would be the model leaking instead — the same distinction `ARCHITECTURE.md` §10 draws between `payment.OrderUpdater.MarkPaid` (an intent) and an ad-hoc from/to status list (the mechanics). `ARCHITECTURE.md` decision 6 rejected `internal/shared/` for the same reason a contract package earns scrutiny: an owned, single-purpose surface with one publisher and named consumers stays legible; an unowned one that answers "what if we just add a field" enough times becomes a second copy of the schema with none of decision 6's ownership discipline.

## Context log attributes are write-only

**Where you hit it:** you want `request_id` in an error response body, or need to forward it as a header on an outbound call.

`logger.WithAttrs` stores a `[]slog.Attr` under an unexported key and only `ContextHandler.Handle` reads it. There is no accessor, and `middleware.GetRequestID` was deleted once nothing needed it. Both uses above need the value itself, not a log record.

**A second, sharper limit: nothing checks the single-naming invariant.** An attribute named at two points on one code path is emitted twice, and slog does not deduplicate keys. `payment/jobs.Dispatcher.Process` names `job_id` for the whole worker path; `charge.UseCase.FinalizePaymentSuccess` and `charge.UseCase.RunCompensatingRefund` are also reached from `charge.UseCase.InitiatePayment` and `webhook.UseCase.Execute`, which pass a `Job` literal with no `ID` and so deliberately name nothing. Add a fourth caller that names `job_id` itself and the worker path start emitting the key twice, with no test or linter to catch it. The check is a grep of the callers, run by hand.

This is not hypothetical. Naming `user_id` at the auth edge immediately collided with an `invalidateStatusCache` helper that logged its own `user_id` — the user being acted upon, not the caller. On an admin role change the record carried both, and a last-wins parser kept the admin's id while silently dropping the target's. The fix was to rename the inner one to `target_user_id`, because the two values answer different questions. That helper is now duplicated three times — `user/usecase/remove/usecase.go`, `user/usecase/adminupdate/usecase.go`, `user/usecase/updaterole/usecase.go` — one private copy per slice that needs it, each logging `target_user_id` the same way; the collision this paragraph describes had to be fixed in all three once slicing split them apart.

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

- `ARCHITECTURE.md` — the sixteen decisions and fifteen rejections these are shadow of.
- `db/OWNERSHIP.md` — table-ownership map, foreign-key inventory, and full blind-spot list for `make check-boundaries`.
- `AGENTS.md` — working rules, and which of them machine-checked.
