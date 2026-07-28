// Package money represents an exact amount in a single currency, so that an
// amount and the currency it is denominated in travel together and cannot drift
// apart.
//
// Everywhere else in this codebase money is still a bare int64 — sometimes with
// a Currency string somewhere alongside it, often without — and nothing enforces
// the pairing. Money makes the pair one value and turns the single mistake that
// actually corrupts a ledger, combining two currencies, into a returned
// [ErrCurrencyMismatch] rather than a plausible-looking wrong number.
//
// Amounts are whole minor units: cents, sen, satang. Never a fraction of one.
//
// The API is deliberately small — [New], [Money.Add], [Money.Sub],
// [Money.MulQty], [Money.Equal], [Money.IsZero] — and two of its absences will
// look like oversights to anyone reading it later. They are not, so they are
// written down here.
//
// # No Div, and no float constructor
//
// Dividing money is not arithmetic, it is policy. Splitting 10 cents three ways
// leaves a remainder, and something has to decide who gets the leftover cent:
// the first party, the last, the largest share, the tax authority. Rounding
// direction is a second decision hiding behind the same operator. A Div method
// would have to pick silently, at which point every caller inherits a policy it
// never chose and cannot see — which is exactly how rounding bugs get into a
// ledger and stay there, a cent at a time, until a reconciliation fails months
// later.
//
// So there is no Div. When a split is genuinely needed, add a method whose name
// states the policy it implements — SplitEvenlyRemainderToFirst, say — and test
// the remainder case explicitly. The name is the documentation, and a caller
// choosing between two such methods is making the decision consciously.
//
// A float constructor is refused for the same reason one step earlier: taking a
// float64 means rounding to minor units on the way in, which is a policy
// decision disguised as a type conversion. Callers that hold a decimal string
// should parse it themselves, where the rounding is visible.
//
// # No MarshalJSON, no sql.Scanner, no driver.Valuer
//
// Money does not know how to serialise itself, because serialisation is not one
// thing here. It is each adapter's job: internal/<x>/postgres maps a Money onto
// its two columns, and internal/<x>/http maps it onto whatever that endpoint's
// contract already promises. This is the payoff from Phase 3 splitting the
// adapters out and Phase 4 moving the wire DTOs into internal/<x>/http — the
// wire shape is owned in one place per endpoint, and it is not here.
//
// The concrete reason, which matters more than the principle: the existing
// responses do not agree with each other about currency, so there is no single
// marshalling that could be correct for all of them.
//
//   - cart's total, dashboard's revenue and total_revenue, and promotion's
//     discount, value and min_order_amount have no sibling currency key at all.
//   - order's line items expose price with no currency on the item — the
//     currency sits on the parent order object instead.
//   - order, payment and product responses already emit their own currency key
//     beside the amount.
//
// A self-marshalling Money would therefore add a key to the first group and
// double-emit it for the third, breaking both in one change. Any future
// [encoding/json.Marshaler] on this type is a wire-contract change to several
// endpoints at once, disguised as a convenience method. The same argument
// applies to [database/sql.Scanner] and [database/sql/driver.Valuer]: a Money
// spans two columns whose names differ per table, so only the repository that
// owns the table can know them.
//
// # What this package does not do
//
// [New] does not validate that currency is a real ISO 4217 code, or even three
// letters. Validating untrusted input belongs at the boundary that received it,
// not in a constructor every internal caller pays for. Two Money values only
// ever compare equal when their currency strings match exactly, so a typo
// surfaces as [ErrCurrencyMismatch] rather than as bad arithmetic.
//
// [Money.Add], [Money.Sub] and [Money.MulQty] do not check for int64 overflow.
// The ceiling is roughly 9.2e18 minor units — about 92 quadrillion currency
// units — which no order in this system approaches. If that ever stops being
// true, the fix is a checked variant, not a silent wrap.
//
// # Two methods with no production caller
//
// [Money.Sub] and [Money.IsZero] are currently unused outside tests, which is
// worth stating rather than leaving as a puzzle for whoever greps for callers.
// Both are kept deliberately: they are the natural complements of [Money.Add]
// and the arithmetic a monetary type is expected to offer, and this package is
// a template others will copy.
//
// Their absence at the two sites you would expect them is also deliberate, and
// instructive. order.Service.PlaceOrder computes a discounted total with plain
// arithmetic — max(subtotal-discount, 0) — rather than Sub, because clamping at
// zero is a policy Sub refuses to choose: an over-large coupon must not produce
// a negative charge, but a refund legitimately is negative, so the caller has to
// say which it means. And it tests Total.Amount > 0 rather than !IsZero(),
// because those differ on a negative total and only the first is the question
// being asked. Two cases where the value object stops short on purpose.
package money
