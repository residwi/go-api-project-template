package money_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/money"
)

func TestAdd_RefusesMixedCurrencies(t *testing.T) {
	usd := money.New(1000, "USD")
	idr := money.New(1000, "IDR")

	_, err := usd.Add(idr)
	require.ErrorIs(t, err, money.ErrCurrencyMismatch,
		"adding across currencies must be impossible, not silently wrong")
}

func TestAdd(t *testing.T) {
	t.Run("adds within one currency", func(t *testing.T) {
		got, err := money.New(1000, "USD").Add(money.New(250, "USD"))
		require.NoError(t, err)
		assert.Equal(t, money.New(1250, "USD"), got)
	})

	t.Run("mismatch error names both currencies", func(t *testing.T) {
		_, err := money.New(1000, "USD").Add(money.New(1000, "IDR"))
		require.EqualError(t, err, "currency mismatch: USD + IDR",
			"the error must say which pair collided, or the log is useless")
	})

	t.Run("mismatch yields the zero value, not a plausible amount", func(t *testing.T) {
		got, err := money.New(1000, "USD").Add(money.New(1000, "IDR"))
		require.Error(t, err)
		assert.Equal(t, money.Money{}, got)
	})
}

func TestSub(t *testing.T) {
	t.Run("refuses mixed currencies", func(t *testing.T) {
		_, err := money.New(1000, "USD").Sub(money.New(1000, "IDR"))
		require.ErrorIs(t, err, money.ErrCurrencyMismatch,
			"subtracting across currencies must be impossible, not silently wrong")
	})

	t.Run("subtracts within one currency", func(t *testing.T) {
		got, err := money.New(1000, "USD").Sub(money.New(250, "USD"))
		require.NoError(t, err)
		assert.Equal(t, money.New(750, "USD"), got)
	})

	t.Run("result may be negative", func(t *testing.T) {
		got, err := money.New(250, "USD").Sub(money.New(1000, "USD"))
		require.NoError(t, err)
		assert.Equal(t, money.New(-750, "USD"), got)
	})
}

func TestMulQty(t *testing.T) {
	// A quantity is dimensionless, so MulQty takes no currency and cannot fail.
	t.Run("scales by quantity", func(t *testing.T) {
		assert.Equal(t, money.New(2997, "USD"), money.New(999, "USD").MulQty(3))
	})

	t.Run("quantity of zero yields zero in the same currency", func(t *testing.T) {
		assert.Equal(t, money.New(0, "USD"), money.New(999, "USD").MulQty(0))
	})

	t.Run("quantity of one is identity", func(t *testing.T) {
		assert.Equal(t, money.New(999, "IDR"), money.New(999, "IDR").MulQty(1))
	})

	t.Run("preserves currency", func(t *testing.T) {
		assert.Equal(t, "IDR", money.New(1500, "IDR").MulQty(4).Currency)
	})
}

func TestEqual(t *testing.T) {
	t.Run("true when amount and currency both match", func(t *testing.T) {
		assert.True(t, money.New(1000, "USD").Equal(money.New(1000, "USD")))
	})

	t.Run("false when amounts differ", func(t *testing.T) {
		assert.False(t, money.New(1000, "USD").Equal(money.New(1001, "USD")))
	})

	// The trap: an implementation that compares only Amount passes every test
	// above. 1000 cents and 1000 rupiah are not the same value.
	t.Run("false when only the currency differs", func(t *testing.T) {
		assert.False(t, money.New(1000, "USD").Equal(money.New(1000, "IDR")),
			"same amount in a different currency is not equal")
	})

	t.Run("false when both differ", func(t *testing.T) {
		assert.False(t, money.New(1000, "USD").Equal(money.New(500, "IDR")))
	})

	t.Run("zero amounts in different currencies are not equal", func(t *testing.T) {
		assert.False(t, money.New(0, "USD").Equal(money.New(0, "IDR")),
			"zero is still denominated; only IsZero ignores the currency")
	})
}

func TestIsZero(t *testing.T) {
	// IsZero asks about the amount, not the currency. Both a denominated zero
	// (money.Money{0, "USD"}) and the zero value of the struct (money.Money{0, ""})
	// are zero, because the question callers actually ask is "is there nothing to
	// charge / discount / refund here", and neither has anything to move. Currency
	// is irrelevant to that question, so IsZero never inspects it.
	t.Run("denominated zero is zero", func(t *testing.T) {
		assert.True(t, money.New(0, "USD").IsZero())
	})

	t.Run("zero value of the struct is zero", func(t *testing.T) {
		assert.True(t, money.Money{}.IsZero())
	})

	t.Run("non-zero amount is not zero", func(t *testing.T) {
		assert.False(t, money.New(1, "USD").IsZero())
	})

	t.Run("negative amount is not zero", func(t *testing.T) {
		assert.False(t, money.New(-1, "USD").IsZero())
	})
}
