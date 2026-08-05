// Package money is the codebase's monetary value object: an amount paired with
// the currency it is denominated in, so the two cannot drift apart.
//
// Amounts are int64 MINOR units. money.New(1999, "USD") is $19.99, not $1999.
// This is the one thing callers get wrong.
//
// Add refuses to combine different currencies, returning ErrCurrencyMismatch
// rather than a silently meaningless number. Equal compares amount AND
// currency. MulQty is the only multiply, because line totals are the only place
// the domain scales money and they always scale by a count.
//
// Deliberate absences, each of which will look like an oversight: there is no
// Sub (nothing subtracts money yet; the coupon clamp works on raw amounts
// because it also needs a max(..., 0) policy this package does not decide), no
// Div (dividing money needs a stated rounding and remainder policy), no float
// constructor, and no MarshalJSON/sql.Scanner/driver.Valuer -- serialisation
// belongs to each adapter, because the existing responses do not agree with each
// other about currency. ARCHITECTURE.md section 10 has the reasoning; do not
// re-derive it here.
//
// New does not validate the currency string, and the arithmetic does not check
// int64 overflow (the ceiling is ~9.2e18 minor units). Both are boundary
// concerns, not constructor concerns.
package money
