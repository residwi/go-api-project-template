# Table ownership

Every table has exactly one owning module. Only that module's `postgres`
adapter may name it in SQL — in a `FROM`, a `JOIN`, an `INSERT INTO`, an
`UPDATE`, a `TRUNCATE` or a `COPY`. Every other module reads it through a
consumer-declared port.

19 tables, 12 owning modules, one owner each. Nothing is unowned and nothing is
shared. `ARCHITECTURE.md` section 6 states the rule and what it costs; this file
is the list, and `scripts/check-boundaries.sh` reads the list from here.

<!--
  PARSING CONTRACT — scripts/check-boundaries.sh parses the table below.

  Between the BEGIN and END markers, every line beginning with `|` is read as

      | `<table>` | `<owning module>` |

  by splitting on `|` and stripping backticks and surrounding whitespace. The
  header row and the `---` separator row are recognised and skipped. Column
  order is fixed: table first, owner second. One table per row.

  Ways to break this by "just tidying it up":
    * Swapping the two columns silently inverts every ownership rule.
    * Putting two tables in one cell — `` `orders`, `order_items` `` — makes the
      whole cell a single unrecognised table name, so both real tables become
      unowned and their module's every query becomes a violation.
    * Moving a row outside the markers removes that table from the check
      entirely, which fails open for its owner and closed for everyone else.

  Rows are sorted by owner, then by table, so the 12-way partition is visible
  at a glance. The script does not depend on the order.

  An unparseable table fails `make check-boundaries` loudly rather than passing
  vacuously. Run it after editing.
-->

<!-- BEGIN OWNERSHIP TABLE -->

| Table | Owner |
| --- | --- |
| `cart_items` | `cart` |
| `carts` | `cart` |
| `categories` | `category` |
| `inventory_levels` | `inventory` |
| `notification_jobs` | `notification` |
| `notifications` | `notification` |
| `order_items` | `order` |
| `orders` | `order` |
| `payment_jobs` | `payment` |
| `payments` | `payment` |
| `product_images` | `product` |
| `products` | `product` |
| `coupon_usages` | `promotion` |
| `promotions` | `promotion` |
| `reviews` | `review` |
| `shipments` | `shipping` |
| `users` | `user` |
| `wishlist_items` | `wishlist` |
| `wishlists` | `wishlist` |

<!-- END OWNERSHIP TABLE -->

## Two modules own no table, for different reasons

`auth` has no `postgres` adapter at all. It needs one thing from storage — look
up a user by email — and it asks for it through `auth.UserDirectory`, implemented
by `user`. Nothing about `auth` is checked here because there is nothing to
check.

`dashboard` is the interesting one, and it is a deliberate exception.

## The reporting carve-out

`dashboard` owns no table and reads `orders` and `order_items` directly, in
read-only aggregate queries. `scripts/check-boundaries.sh` exempts it by name.

**What it buys.** A revenue-by-day figure is one `GROUP BY` over two joined
tables. Expressed through ports it would be a page of orders, then a call per
order for its items, then summation in Go — slower, and less correct, because
the numbers would be assembled from reads taken at different instants. Reporting
also participates in no module's invariants: there is nothing for a port to
protect, because `dashboard` never writes.

**What it costs.** Three things, and none of them are hypothetical:

1. `dashboard` is coupled to another module's *column* names, not just its
   tables. Rename `order_items.unit_price` and `dashboard` breaks. Nothing
   catches it: `go build` cannot see inside a SQL string, and the boundary check
   exempts `dashboard` wholesale. The failure surfaces at runtime, in a query
   only an admin runs, which is close to the worst place to learn about it.
2. The carve-out has no stated bound. "May read anything" means today's two
   tables can become twelve without a single review comment, at which point
   `dashboard` is a second, undeclared copy of the schema. If that happens the
   answer is a real read model — a projection `dashboard` owns and other modules
   write to — not a wider exemption.
3. Read-only is a convention here, not a constraint. No grant, no separate role,
   and no check enforces it. An `UPDATE` in `internal/modules/dashboard/adapter/postgres` would
   pass CI today.

## Cross-module foreign keys are kept

25 foreign keys exist. 18 of them cross a module boundary. All 18 stay.

The 7 that do not cross are aggregate-internal and unremarkable:
`cart_items→carts`, `categories→categories` (the parent link),
`coupon_usages→promotions`, `order_items→orders`, `payment_jobs→payments`,
`product_images→products`, `wishlist_items→wishlists`.

The 18 that do cross, in full — because "step one of any split is dropping 18
constraints" is not actionable without the list:

| Referencing table | → | Referenced table | Crosses |
| --- | --- | --- | --- |
| `cart_items` | → | `products` | cart → product |
| `carts` | → | `users` | cart → user |
| `coupon_usages` | → | `orders` | promotion → order |
| `coupon_usages` | → | `users` | promotion → user |
| `inventory_levels` | → | `products` | inventory → product |
| `notification_jobs` | → | `users` | notification → user |
| `notifications` | → | `users` | notification → user |
| `order_items` | → | `products` | order → product |
| `orders` | → | `users` | order → user |
| `payment_jobs` | → | `orders` | payment → order |
| `payments` | → | `orders` | payment → order |
| `products` | → | `categories` | product → category |
| `reviews` | → | `orders` | review → order |
| `reviews` | → | `products` | review → product |
| `reviews` | → | `users` | review → user |
| `shipments` | → | `orders` | shipping → order |
| `wishlist_items` | → | `products` | wishlist → product |
| `wishlists` | → | `users` | wishlist → user |

All 18 are `NO ACTION`. The only 4 `ON DELETE CASCADE` constraints in the schema
are within-module: `cart_items→carts`, `order_items→orders`,
`product_images→products`, `wishlist_items→wishlists`.

**What they buy.** Postgres refuses an `order_items` row pointing at a product
id that never existed. A port cannot give you that: it checks at a different
moment than the one the write commits in, so between the check and the insert
there is a window. The constraint has no window.

**What they cost.** They are precisely what makes extracting a module into a
service a *data* problem rather than a code problem. You cannot put `orders` and
`products` in separate databases while `order_items.product_id` carries a
foreign key. Step one of any split is dropping 18 constraints and re-expressing
each as an application-level check with an explicit answer for the race the
constraint used to close — a migration with a correctness argument attached, not
a refactor. Every port declared during this refactor makes the *code* side of
that split cheap; none of them touch this side.

One of the 18 is load-bearing in Go rather than merely defensive.
`categories` is the only hard-deleted table that another module's table
references, so `products.category_id` is the one cross-module constraint that
can actually fire in normal operation. `category`'s adapter catches the
violation and turns it into a domain error, as a backstop behind
`category.ProductCounter`'s friendlier pre-check. Delete that constraint and the
backstop goes with it.

## The FK graph is not the dependency graph

Inbound foreign keys, by referenced table — every referenced table, so the
column sums to all 25 rather than to a selection:

| Table | Inbound FKs | Inbound ports |
| --- | --- | --- |
| `users` | 7 | 1 |
| `orders` | 6 | 7 |
| `products` | 6 | 2 |
| `categories` | 2 | — |
| `carts` | 1 | — |
| `payments` | 1 | — |
| `promotions` | 1 | — |
| `wishlists` | 1 | — |
| `inventory_levels` | 0 | 5 |
| **total** | **25** | |

The four tables with a single inbound FK each have it from their *own* module's
child table, which is why they carry no ports: nothing outside asks them
anything.

("Inbound ports" counts interfaces other modules declare that this module's
service satisfies — `auth.UserDirectory`, `payment.OrderGetter`,
`product.InventoryReader` and so on.)

It is tempting to read the first column as a module dependency ranking. It is
not one. `users` is the most-referenced table in the schema and almost nothing
calls into `user`: seven tables carry a `user_id`, and a caller writing one
already has the id, so it has nothing to ask. `inventory_levels` has no inbound
foreign keys at all and five interfaces across three modules (`order`,
`payment`, `product`) declare ports against `inventory`, because
stock is an answer that changes and must be asked for every time.

Foreign-key fan-in measures how many tables carry an identity. Port fan-in
measures how much *behaviour* other modules need. `orders` is the only table
high on both, which is why `order` is the module that would be hardest to
extract and the one to be most careful with.

## Cross-module `ON DELETE CASCADE` is not kept

`db/migrations/20260424120016_drop_cross_module_cascades.sql` dropped six
cascades, keeping each reference: `carts.user_id`, `cart_items.product_id`,
`wishlists.user_id`, `wishlist_items.product_id`, `notifications.user_id`,
`notification_jobs.user_id`.

They were unreachable. `users` and `products` are soft-deleted — `UPDATE ... SET
deleted_at` — so no `DELETE` ever reaches those rows and the cascade could never
fire. Meanwhile the schema advertised a cart and wishlist cleanup that nothing
performed, and it described the database reaching across a module boundary to
write, which no port describes. `ARCHITECTURE.md` decision 8: a lie in the
schema is worse than an absence.

**What it costs.** Nothing cleans up a soft-deleted user's cart — though nothing
ever did, which is the point. If hard deletes are ever introduced, that cleanup
has to be written as an explicit cross-module operation: a port call, or a job.
That is the honest shape, but it is work the cascade appeared to have already
done, and whoever adds the first hard delete has to notice.

Four cascades remain, all within a module and all correct, because they are
aggregate-internal: `cart_items→carts`, `order_items→orders`,
`product_images→products`, `wishlist_items→wishlists`. Deleting a cart really
does mean deleting its lines.

## Enforcement

`make check-boundaries` runs `scripts/check-boundaries.sh`, which registers
seven checks. Two of them read the table above, straight out of this
document at run time, so this document is the source of truth and not a
copy of one: check 2 (`check_ownership_doc`) validates the table itself —
no duplicate row, no row for a table no migration creates, no table with
no owning row — and check 3 (`check_table_ownership`) is the one described
below, enforcing what each module's own SQL may name. Change ownership
here; there is no list in the script to keep in step.

**What it catches.**

* A production query naming a table the module does not own, via `FROM`,
  `JOIN`, `INSERT INTO`, `UPDATE`, `TRUNCATE` or `COPY`, anywhere under the
  module — every non-test `.go` file under `internal/modules/<module>/`, not
  only the ones inside a directory named `postgres`. The scan used to be
  scoped to `postgres/` directories only, which meant a query in `service.go`
  or a slice's `usecase.go` was invisible; there is no longer a privileged
  directory, so the whole module is scanned. A module is skipped only when
  it has no `postgres/` directory anywhere under it — a legitimate no-storage
  feature, e.g. `auth`. Only a match against a table actually listed in this
  document is reported: the identifier a keyword is followed by must be a
  real table name, not merely a word absent from the scanning module's own
  list, or every `slog` call shaped like `"failed to update payment
  status"` — common once service and command files are in scope — would
  report a violation on every run. `DELETE FROM` and `MERGE INTO` come along
  through `FROM` and `INTO`.
* The same, when the keyword and the table name are on different lines.
  Whitespace is collapsed across newlines before matching, so
  `INSERT INTO\n    products (...)` is caught. It was not, before Phase 5.
* The same, when the table is written as a quoted identifier: `FROM "products"`.
* A CTE named after a real table — `WITH orders AS (...)` in, say,
  `internal/modules/payment/usecase/webhook/postgres/`. This is refused rather than exempted, because
  exempting it hid every genuine reference to `orders` in that file, reads and
  writes alike, without anyone touching this document. Per-statement CTE scoping
  would not have been enough: SQL says a non-recursive CTE body does not see the
  CTE, so `WITH orders AS (SELECT id FROM orders ...)` reads the real table from
  inside the very statement that declares the name.
* A module that has a `postgres` adapter and no row in this file.
* A table recorded here that no `db/migrations/*.sql` creates, and a table a
  migration creates with no owner recorded here. Both directions, because a
  stale row quietly widens an allowlist and a missing row narrows one.
* The same table listed twice, whether or not the two rows agree.

**What it does not catch.** This matters more than the list above, because a
check trusted past its reach is worse than no check.

* **`dashboard`, at all.** Exempt by name, per the carve-out. Nothing verifies
  it stays read-only or stays at two tables.
* **Table names that are not literals.** The check is a grep for the identifier
  after a SQL keyword. Every query today has its table name in the string
  literal, but `fmt.Sprintf` is already routine in these adapters for `WHERE`
  fragments and placeholder lists, so the habit of assembling SQL exists — it
  simply has not reached a table name yet. The day it does, the check goes quiet
  rather than failing. `pgx.CopyFrom` is the same hole with no `Sprintf` in
  sight: it names its table as a `pgx.Identifier{"orders"}` Go value, so no SQL
  keyword precedes it and nothing here sees it. Nothing uses it today.
* **Prose it mistakes for SQL — a false positive, not a false negative.** Go
  `//` and SQL `--` comments are stripped, and `_test.go` files are skipped,
  which between them removed most of it. What remains is prose in a *production*
  string literal: `var msg = "update orders failed"` in, say,
  `internal/modules/cart/usecase/remove/postgres/` reports `orders`. Nothing available to a grep can
  tell that string from a query. It fails loudly rather than silently, so the
  cost is an afternoon of confusion, not a boundary crossing — but if it starts
  happening often the answer is a SQL parser, not a wider allowlist.
* **Test files, deliberately.** A test seeds sibling tables to satisfy foreign
  keys, and that is fixture setup, not an architectural crossing. The cost is
  that the check cannot distinguish a fixture from a real violation that happens
  to live in a `_test.go` helper.
* **`db/seeds/data.sql` and `db/migrations/*.sql`.** This check only reads
  `.go` files, so raw SQL outside Go source is never scanned. This is
  unrelated to which directory under a module gets scanned (see "What it
  catches" above) — a `.sql` file carries no Go package for a violation to
  be attributed to, so widening which `.go` files are read cannot reach it.
* **Column-level coupling.** Ownership is per table. `dashboard` depending on
  `order_items.unit_price`, or any module depending on a column it does not
  control, is invisible to a table-name grep even where the table is allowed.
* **Anything the database does on its own.** A view, trigger or function that
  reads across modules is not Go source and is not scanned.
