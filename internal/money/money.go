package money

import (
	"errors"
	"fmt"
)

// ErrCurrencyMismatch is returned when an operation combines two Money values of different currencies.
var ErrCurrencyMismatch = errors.New("currency mismatch")

// Money is an exact amount in a single currency.
type Money struct {
	// Amount is in minor units (cents, sen, satang) — never a fraction of one.
	Amount int64
	// Currency is an ISO 4217 alphabetic code, three letters, e.g. "USD".
	Currency string
}

// New returns a Money of amount minor units in currency.
func New(amount int64, currency string) Money {
	return Money{Amount: amount, Currency: currency}
}

// Add returns the sum of m and other, or [ErrCurrencyMismatch] if their currencies differ.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}

	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

// MulQty returns m scaled by qty, keeping m's currency.
//
// A quantity is dimensionless — three of something is three whatever the price
// is denominated in — so there is no currency to disagree about and MulQty
// cannot fail. Scaling by an integer is also exact, which is why this and not a
// general multiply is the operation the package offers: line totals are the only
// place the domain multiplies money, and they always multiply by a count.
func (m Money) MulQty(qty int) Money {
	return Money{Amount: m.Amount * int64(qty), Currency: m.Currency}
}

// Equal reports whether m and other are the same amount in the same currency.
//
// Both fields must match. Comparing only Amount would make 1000 cents equal
// 1000 rupiah, which is the same class of bug [ErrCurrencyMismatch] exists to
// prevent — so Equal is deliberately not a shorthand for "same number".
func (m Money) Equal(other Money) bool {
	return m.Amount == other.Amount && m.Currency == other.Currency
}

// IsZero reports whether m's amount is zero; the currency is not consulted.
func (m Money) IsZero() bool {
	return m.Amount == 0
}
