// Package money pairs an amount with the currency it is denominated in, so the
// two cannot drift apart.
//
// Amounts are int64 MINOR units. money.New(1999, "USD") is $19.99, not $1999.
// This is the one thing callers get wrong.
//
// Deliberate absences, each of which will look like an oversight: no Sub
// (nothing subtracts yet, and the coupon clamp needs a max(..., 0) policy this
// package does not decide), no Div (needs a stated rounding and remainder
// policy), no float constructor, and no MarshalJSON/sql.Scanner/driver.Valuer,
// because the existing responses disagree about currency and serialisation is
// each adapter's call. ARCHITECTURE.md section 10 has the reasoning.
//
// New does not validate the currency string and the arithmetic does not check
// int64 overflow (~9.2e18 minor units). Both are boundary concerns.
package money
