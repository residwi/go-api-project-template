# Refactor plan: collapse slices into per-module services

Status: agreed, not started.

This plan replaces the vertical-slice layout described in `CLAUDE.md` ("Inside a
feature") with one service per module plus an `adapter/` subtree. It also extracts
a `checkout` module, moves the transport layer to `internal/server/`, moves
`money` under `internal/modules/`, and renames `testhelper` to `testutil`.

Every number below was measured against the tree at the time of writing. Re-measure
before trusting one.

## Why

The slice layout duplicates more than it isolates.

| Measure                                               | Today       |
| ----------------------------------------------------- | ----------- |
| Slice directories under `internal/modules/*/usecase/` | 66          |
| `postgres` packages                                   | 64          |
| `http` packages                                       | 53          |
| `ports.go` files                                      | 26          |
| Generated mock code                                   | 28,437 LOC  |
| Non-test module code                                  | ~17,000 LOC |

Generated mocks are 1.66x the production code they test. The cause is that each
slice re-declares the ports its siblings already declared: `order` holds 29 port
interfaces, `payment` 22. Duplicated declarations mean duplicated mocks and
duplicated SQL.

Byte-identical `GetByID` implementations exist 4x in `order`, 4x in `payment` and
6x in `user`. `ListItemsByOrderID` exists 4x in `order`. Deduplicating repository
methods takes `order` from 19 to 13, `payment` 15 to 10, `user` 17 to 9 — about
26 duplicate methods across the tree, each carrying its own mock and its own SQL.

## Target shape

```text
internal/modules/<feature>/
  domain/              aggregate types and rules — stays a package
  contract.go          published types, in the module's root package
  ports.go             cross-module ports, declared by the consumer
  repository.go        ONE Repository interface holding every method
  service.go           ONE Service; Deps and New live here
  config.go            only where the module owns env vars
  adapter/http/        handlers; role-named port per handler
  adapter/postgres/    repository implementation
  adapter/jobs/        payment only — the job dispatcher
  jobs.go              payment only — the queue, in the root package

internal/modules/checkout/   place, retrypayment, cancel orchestration
internal/modules/money/      the Money value object
internal/server/             middleware/ response/ routes.go server.go
internal/testutil/           was internal/testhelper
```

`module.go` is deleted everywhere: with `bootstrap` constructing adapters and one
`Service` per module, `Deps` and `New` belong in `service.go` and nothing is left
for it.

### Decisions and their reasons

**`domain/` stays a package.** 709 LOC across 14 modules. Flattening it into the
root package collides with `contract.go` in five modules — `Claims`, `Cart`,
`Order`, `Product` and `User` each exist as both a domain type and a contract
type. Keeping the package costs one directory and avoids five renames. It also
keeps `order/domain/transition.go` — the 13 named transitions — in one place.

**`contract.go` lives in the module's root package.** A consumer therefore imports
the whole producer module. This is deliberate. It widens the visible surface
(`payment` will be able to see `order.Place`) but it does not weaken any
invariant: `Service`'s repository field is unexported, so every exported method
still routes through the guarded transition table. Go's own visibility rules do
the work that the `contract/` package boundary used to do.

**One `Repository` interface per module, holding every method.** 13 methods for
`order`, 10 for `payment`, 9 for `user`. Mockery's expecter only asserts calls a
test sets up, so unused methods cost a test nothing. The tradeoff is
discoverability: a test using two methods reads a 13-method interface.

**`adapter/http` keeps a role-named port per handler.** `OrderPlacer`,
`OrderReader`, `AdminReader` — the convention in today's rule 18a survives. The
concrete `*Service` is not passed to handlers. This keeps handler tests one layer
deep: forcing a 404 stubs the port, not the repository. It also keeps handler
tests from coupling to the service's error mapping. The cost is that the ~6,606
LOC of http mocks does not disappear; it shrinks only because 53 packages collapse
to about 15, merging their duplicate ports.

**`bootstrap` constructs the postgres adapters; `New` takes a `Repository`.**
Seven repository methods take a package-local param struct (`AdminListParams` in
order, payment and product; `Params` in user and promotion;
`DeliveredPurchaseParams` in order; `PublishedListParams` in product). Flattened,
those structs live in the module's root package, so `adapter/postgres` must import
the root package to implement `Repository` — which means the root package can never
import its own adapter. `Deps.Pool` therefore becomes `Deps.Repo` in all modules.
A side benefit: a module stops importing `pgxpool`, and service tests need no
container.

**payment's queue lives in `payment/jobs.go`, in the root package; only the
dispatcher goes in `adapter/jobs/`.** The service enqueues jobs (outbound) and the
runner calls the service (inbound). Putting both sides in `adapter/jobs` would make
the root package and the adapter import each other. With the queue in the root
package, `Service` calls it directly with no interface between them, and the
`JobEnqueuer` port disappears. The queue's SQL goes to `adapter/postgres`.

**The stale-order sweep moves to `cmd/worker/main.go`** as a small
`jobs.Sweeper`. It is cross-module composition — payment's dispatcher plus order's
`Expire` and `RecoverStale` — and the composition root already exists there. This
deletes `internal/modules/payment/worker/` (47 LOC plus 167 LOC of mocks).

**`money` moves to `internal/modules/money/`,** on the grounds that it is a domain
value object with arithmetic rather than infrastructure. It is 37 LOC and imports
nothing internal. `scripts/check-boundaries.sh` derives its feature list by listing
`internal/modules/`, so `money` needs a name exemption in check 4 or all 43
importers fail the boundary run.

`internal/apperror` deliberately does **not** move. It is a dependency-free
vocabulary package like `money`, but it is not a value object, and the distinction
is what justifies moving one and not the other. State it in `CLAUDE.md` so the next
reader does not have to ask.

**`internal/transport/http/` becomes `internal/server/`,** with all 64 route
registrations in one `routes.go`. The 14 files in `routes/` total 289 LOC; one file
with one `adapter/http` package per module lands around 180, since the per-feature
function signatures and duplicated imports go away.

## Naming

Flattening forces a naming pass. It is not cosmetic: several names only work
today because the package name carries half their meaning, and that package
disappears.

### 37 `Execute` methods must be renamed

`place.UseCase.Execute` reads correctly because the package says `place`. One
`Service` per module cannot hold five `Execute` methods. Every one takes the verb
its directory used to carry:

| Module       | Was                                                                                    | Becomes                                                |
| ------------ | -------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| auth         | `login.Execute`, `refresh.Execute`, `register.Execute`                                 | `Login`, `Refresh`, `Register`                         |
| cart         | `add.Execute`, `remove.Execute`, `updatequantity.Execute`                              | `Add`, `Remove`, `UpdateQuantity`                      |
| category     | `create.Execute`, `update.Execute`, `remove.Execute`                                   | `Create`, `Update`, `Delete`                           |
| inventory    | `adjust.Execute`, `restock.Execute`                                                    | `Adjust`, `Restock`                                    |
| notification | `markread.Execute`, `markallread.Execute`                                              | `MarkRead`, `MarkAllRead`                              |
| order        | `changestatus.Execute`                                                                 | `ChangeStatus`                                         |
| payment      | `refund.Execute`, `webhook.Execute`                                                    | `Refund`, `HandleWebhook`                              |
| product      | `create.Execute`, `update.Execute`, `remove.Execute`                                   | `Create`, `Update`, `Delete`                           |
| promotion    | `create.Execute`, `update.Execute`, `remove.Execute`, `apply.Execute`                  | `Create`, `Update`, `Delete`, `Apply`                  |
| review       | `create.Execute`, `remove.Execute`                                                     | `Create`, `Delete`                                     |
| shipping     | `create.Execute`, `deliver.Execute`, `updatetracking.Execute`                          | `Create`, `Deliver`, `UpdateTracking`                  |
| user         | `updateprofile.Execute`, `updaterole.Execute`, `adminupdate.Execute`, `remove.Execute` | `UpdateProfile`, `UpdateRole`, `AdminUpdate`, `Delete` |
| wishlist     | `add.Execute`, `remove.Execute`                                                        | `Add`, `Remove`                                        |
| checkout     | `place.Execute`, `retrypayment.Execute`, `cancel.Execute`                              | `PlaceOrder`, `RetryPayment`, `CancelOrder`            |

One collision to watch: `product.manageimages` exports `Add` and `Delete`, which
would clash with `create`/`remove` becoming `Create`/`Delete`. Rename those to
`AddImage` and `DeleteImage`.

### Entity modules imply the object; process modules name it

`category.Create` and `order.Get` read correctly because the receiver names the
entity being acted on, so repeating it in the method would stutter.

`checkout` names a process, not an entity. Nothing about the receiver says what is
being placed or cancelled, and `checkout.Create` would read as "create a
checkout" — which is not what the method does. So checkout's methods carry their
object: `PlaceOrder`, `CancelOrder`, `RetryPayment`.

`Place` rather than `Create` for the same reason `Create` is wrong at the module
level: the operation locks the cart, validates it, reserves inventory, reserves a
coupon, writes the order and its items, initiates payment and clears the cart.
`Create` is CRUD vocabulary for a row insert and under-describes all of that.
"Place an order" is the domain term the codebase already uses.

### Method names must not repeat the module

The receiver already says the module, so the method should not.

| Was                              | Becomes                    |
| -------------------------------- | -------------------------- |
| `payment.InitiatePayment`        | `payment.Charge`           |
| `payment.FinalizePaymentSuccess` | `payment.FinalizeSuccess`  |
| `payment.RunCompensatingRefund`  | `payment.CompensateRefund` |
| `cart.GetCart`                   | `cart.Get`                 |
| `order.RecoverStaleProcessing`   | `order.RecoverStale`       |
| `order.GetByIDForUser`           | `order.GetForUser`         |
| `order.GetByID`                  | `order.Get`                |
| `shipping.GetByOrderIDForUser`   | `shipping.GetForUser`      |
| `wishlist.ListItemsForUser`      | `wishlist.List`            |
| `notification.ListByUser`        | `notification.List`        |

`payment.Charge` then sits beside `payment.ProcessCharge`, which is the worker's
entry point for a claimed job — two similar names for genuinely different callers.
Rename the job pair `RunChargeJob` and `RunRefundJob` so the worker entry points
read as worker entry points.

`GetForUser` and `Get` as a pair is the convention: the `ForUser` suffix marks the
method that performs an ownership check, and the plain one does not. Keep it
consistent between `order` and `shipping`.

### Delete two overlapping projections

`order.GetSnapshot` and `order.GetInfo` both return `contract.Order` from the same
`getByID` read. `GetSnapshot` populates the coupon code and totals; `GetInfo` fills
only `ID`, `UserID` and `Status`. Two names that do not say which is which, for one
type. `shipping` consumes `GetInfo`, and everything it needs is in the fuller
value. Delete `GetInfo`, keep one method named `Snapshot`, and point shipping at
it.

### Drop the `Batch` suffix, and the dead singular methods

Verified dead — no caller anywhere, including their own handlers and e2e:

- `inventory.Deduct(ctx, productID, qty)`
- `inventory.Reserve(ctx, productID, qty)`
- `inventory.Release(ctx, productID, qty)`
- `product.AvailableQuantity`

`product.GetByIDsIncludingDeleted` is exported but used only inside its own
package — unexport it.

The three inventory singulars are the only reason `DeductBatch` and `ReserveBatch`
carry a suffix. Delete them, then rename `DeductBatch` to `Deduct` and
`ReserveBatch` to `Reserve`. `Restore` already has no suffix. Their repository
counterparts go too.

Run the same check per module during its flatten. A repo-wide grep is unreliable
because method names collide across modules (`Reserve`, `Release`, `Execute`), so
do it one module at a time where the receiver type is unambiguous.

### Port interfaces: merge duplicates, name for the role

One `ports.go` per module means the duplicate declarations merge:

| Interface                             | Declared                                          | After                                  |
| ------------------------------------- | ------------------------------------------------- | -------------------------------------- |
| `TransitionApplier`                   | 4x in order (cancel, expire, place, recoverstale) | 1                                      |
| `ProductLookup`                       | 3x in cart (add, query, updatequantity)           | 1, union of `GetInfo` + `GetInfoByIDs` |
| `StatusInvalidator`                   | 3x in user (adminupdate, remove, updaterole)      | 1                                      |
| `JobStore`                            | 3x in payment (charge, refund, webhook)           | 1                                      |
| `Gateway`                             | 2x in payment (charge, refund)                    | 1, `Charge` + `Refund`                 |
| `OrderGetter`                         | 2x in shipping (create, query)                    | 1                                      |
| `InventoryRestorer`, `CouponReleaser` | 2-3x each                                         | 1 each                                 |

Four ports are named for the pattern rather than the role, which rule 18a already
forbids for handler ports and should apply here too:

| Was                    | Becomes          |
| ---------------------- | ---------------- |
| `auth.UserPorts`       | `UserDirectory`  |
| `cart.ProductPorts`    | `ProductLookup`  |
| `order.CouponPort`     | `CouponReserver` |
| `order.TransitionPort` | `StatusUpdater`  |

Keep splitting a port per capability where the consumer only needs one method —
`CartLocker`, `CartReader`, `CartClearer` stay three interfaces, not one
`CartPorts`. That is what lets a name-match wire a single slice value, and it
survives the flattening unchanged.

### Handler struct fields

The `UseCase` type name disappears everywhere, so the field that held it must too.
The port keeps its role name; the field and the constructor parameter are
`service`:

```go
// internal/modules/order/adapter/http/handler.go
type OrderReader interface {
	Get(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	List(ctx context.Context, p ListParams) ([]domain.Order, error)
}

type Handler struct {
	service   OrderReader
	validator *validator.Validator
}

func NewHandler(service OrderReader, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}
```

`h.usecase.Execute(...)` becomes `h.service.Place(...)` in all 56 call sites.
Never `svc`, `uc` or `s` — the field is `service` everywhere so a reader moving
between handlers never has to check.

`Repository`, `Deps` and `Service` keep their names. `UseCase` is retired as a
type name; the directory that justified it is gone.

## The checkout module

The order/payment cycle is the reason slice-granularity construction exists.
`bootstrap.New` builds three order slices standalone — `transition`, `query` and
`cancel` — before it can build `payment`, and only then builds `order`. One
`Service` per module makes that ordering impossible: `payment.Service` would need
`order.Service` and vice versa.

`checkout` breaks it by taking the order-to-payment edges out of `order`:

| Use case                                                        | Moves to checkout  | Why                                                 |
| --------------------------------------------------------------- | ------------------ | --------------------------------------------------- |
| `place`                                                         | yes                | needs `payment.InitiatePayment`                     |
| `retrypayment`                                                  | yes                | needs `payment.InitiatePayment`                     |
| `cancel` (user-initiated)                                       | orchestration only | needs `payment.CancelPendingByOrderID`, best-effort |
| `webhook`                                                       | no                 | see below                                           |
| `charge`, `refund`, `query`, `jobs`, `gateway`                  | no                 | payment-to-order only                               |
| `expire`, `changestatus`, `recoverstale`, `transition`, `query` | no                 | no payment edge                                     |

`changestatus` mentions payment only in status names and an error string, not in a
dependency. `expire` needs transition, inventory and coupons — no payment.

`payment/usecase/webhook` stays in payment. `cancel` has two entry points:
`CancelUnpaid` does reversal only and never touches payment — that is the one the
webhook calls — while `Execute` adds a best-effort payment-job cancel behind an
`if c.paymentCancel != nil` guard that exists solely because `app.go:77` builds a
second, crippled instance for payment. Splitting the two lets the webhook call
`order.CancelUnpaid` directly, keeps payment's `payments`-table repository inside
payment, and deletes the nil guard.

So `order` exposes `CancelByUser` (ownership check, charging guard, reversal) and
`checkout.CancelOrder` is a ~15-line wrapper calling that plus payment's job cancel.

**checkout owns no tables.** `place` currently inserts `orders` and `order_items`;
after the move it calls order ports instead. One writer per aggregate, so
`db/OWNERSHIP.md` is unchanged and check 3 keeps working. checkout gets no
`postgres` adapter, no `domain/`, no `contract.go` (nothing imports it) and no
`config.go`.

Ports checkout declares in its own `ports.go`:

- order: `Create`, `CreateItems`, `UpdateTotals`, `GetByID`,
  `GetByUserIDAndIdempotencyKey`, `ListItemsByOrderID`, the transition intents,
  `CancelByUser`
- payment: `InitiatePayment`, `CancelPendingByOrderID`
- cart: `Lock`, `Snapshot`, `Clear`
- inventory: `ReserveBatch`, `DeductBatch`, `Restore`
- promotion: `Reserve`, `Release`
- notification: `EnqueueOrderPlaced`

Resulting sizes, use-case logic only: `checkout` ~286 LOC, `order` ~453,
`payment` ~822.

The dependency graph is then acyclic:

```text
checkout -> order, payment, cart, inventory, promotion, notification
payment  -> order, inventory, promotion
shipping -> order
review   -> order
auth     -> user
cart     -> product
product  -> inventory
category -> product
```

## What this gives up

Check 3 (`check_table_ownership`) survives intact, because checkout owns no
tables. Rule 14's guard on order status survives, because `Service`'s repository
field is unexported.

What goes is **compile-enforced module privacy**. Today `payment` cannot name
`order.Service` at all; check 4 fails the build. After the refactor it can see
every exported method on it. This is a widened surface, not a bypass — but it is
a real loss, and it is the one accepted cost of `contract.go` living in the root
package.

Three checks in `scripts/check-boundaries.sh` become dead or need rewriting:

- check 4 (`check_cross_module_imports`) — module roots become importable, so this
  check either goes away or narrows to "no importing another module's `domain/` or
  `adapter/`"
- check 5 (`check_sibling_slice_imports`) — there are no slices left
- check 7 (`check_contract_leaf`) — `contract.go` is in the root package, which
  imports `domain/` by definition

check 1 (`check_wire_tags`) needs its exempt path changed from
`internal/modules/<feature>/usecase/<slice>/http/` to
`internal/modules/<feature>/adapter/http/`. Check 2, 3 and 6 are unaffected.

## Sequence

Each step is a separate branch, rebased onto `main` and merged fast-forward.

**0. Route snapshot test.** 64 routes are registered; `router_test.go` asserts 19.
Forty-five routes have nothing proving which group they land on, and step 4 moves
all 64 into one file. Write a table test asserting every `method path -> group`
pair before touching anything. About 70 LOC. This is the only safety net for the
transport move; `test/e2e` (2,187 LOC) covers the sagas, not the route table.

**1. Extract `checkout`.** Do this on today's slice shape, where it is a move of
whole directories. Split `cancel` into `order.CancelByUser` plus
`checkout.CancelOrder`, point `payment/webhook` at `order.CancelUnpaid`, delete the
`!= nil` guard, and delete the duplicate `ordercancel` construction at
`app.go:77`. This kills the order/payment cycle, which is what makes every later
step possible.

**2. Flatten module by module,** simplest first, merging each before starting the
next:

`wishlist` (3 slices, 481 LOC) to prove the shape, then `review`, `notification`,
`dashboard`, `category`, `shipping`, `inventory`, `promotion`, `cart`, `user`,
`product`, and `order`, `payment`, `checkout` last.

Per module: merge the slice repository interfaces into one `repository.go`,
deduplicating; merge the slice `postgres` packages into one `adapter/postgres`,
deleting duplicate SQL; merge the slice `http` packages into one `adapter/http`,
merging duplicate ports; merge the `usecase.go` files into one `service.go`;
merge `ports.go`; delete `module.go`; change `Deps.Pool` to `Deps.Repo` and move
adapter construction to `bootstrap`; move `contract/` to `contract.go`; run
`make mocks`; run `make all`.

Then the naming work for that module, from the Naming section above: rename its
`Execute` methods, strip module-name stutter, merge duplicate port interfaces,
rename any pattern-named port, change every handler's `usecase` field to
`service`, and sweep for methods with no caller now that the receiver type is
unambiguous.

Keep test files split per use case — `service_place_test.go`,
`service_cancel_test.go` — since Go allows many test files per package and
`order`'s use-case tests alone are 2,180 LOC.

Two module shapes coexist in the tree for the duration. Note that in `CLAUDE.md`
while it is true.

**3. `internal/modules/money`,** plus the exemption in
`scripts/check-boundaries.sh`.

**4. `internal/transport/http/` to `internal/server/`,** collapsing `routes/`
into one `routes.go`. **Delete all 14 files in `internal/transport/http/routes/`
and the directory itself** — they are replaced, not kept alongside. `router.go`'s
group and rate-limiter setup moves into `server.go`; the 14
`routes.<Feature>(...)` calls become inline registrations in `routes.go`. The
snapshot test from step 0 is what proves nothing moved groups.

**5. `internal/testhelper` to `internal/testutil`.** Pure rename.

**6. Rewrite `CLAUDE.md` and `ARCHITECTURE.md`.** The `usecase/` shape, rule 10,
rule 13, rule 18a's scope and checks 4, 5 and 7 all change or die. Update
`scripts/check-boundaries.sh` in the same commit as the rules it enforces.

## Verification

`make all` after every module. `make ci` does not run `check-boundaries`, so run
it explicitly or use `make all`.

`test/e2e` and the step-0 route snapshot are the two things that can catch a
regression the per-module tests cannot: e2e for the sagas that cross modules,
the snapshot for the route table. Neither should be skipped on a "mechanical"
step.
