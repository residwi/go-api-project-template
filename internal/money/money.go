package money

import (
	"errors"
	"fmt"
)

// ErrCurrencyMismatch is returned by operations that combine two Money values
// of different currencies. Wrapped errors name both operands, so callers can
// match with [errors.Is] and still log which pair collided.
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

// Add returns the sum of m and other. It fails with [ErrCurrencyMismatch]
// unless both operands share a currency.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}

	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

// Sub returns m minus other. It fails with [ErrCurrencyMismatch] unless both
// operands share a currency. The result may be negative: a refund, a credit and
// an over-payment are all legitimate negative amounts, so Sub does not clamp.
func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s - %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}

	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}, nil
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

// IsZero reports whether m has an amount of zero.
//
// This is a question about the amount only, so the currency is not consulted:
// both a denominated zero (Money{0, "USD"}) and the zero value of the struct
// (Money{0, ""}) report true. Callers ask IsZero to decide whether there is
// anything to charge, discount or refund, and in neither case is there. Use
// [Money.Equal] instead when the currency must also match.
func (m Money) IsZero() bool {
	return m.Amount == 0
}
