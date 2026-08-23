package money

import (
	"errors"
	"fmt"
)

var ErrCurrencyMismatch = errors.New("currency mismatch")

type Money struct {
	Amount   int64
	Currency string
}

func New(amount int64, currency string) Money {
	return Money{Amount: amount, Currency: currency}
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}

	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

func (m Money) MulQty(qty int) Money {
	return Money{Amount: m.Amount * int64(qty), Currency: m.Currency}
}

func (m Money) Equal(other Money) bool {
	return m.Amount == other.Amount && m.Currency == other.Currency
}

func (m Money) IsZero() bool {
	return m.Amount == 0
}
