package money

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdd_RefusesMixedCurrencies(t *testing.T) {
	t.Parallel()

	usd := New(1000, "USD")
	idr := New(1000, "IDR")

	_, err := usd.Add(idr)
	require.ErrorIs(t, err, ErrCurrencyMismatch,
		"adding across currencies must be impossible, not silently wrong")
}

func TestAdd(t *testing.T) {
	t.Parallel()

	t.Run("adds within one currency", func(t *testing.T) {
		t.Parallel()

		got, err := New(1000, "USD").Add(New(250, "USD"))
		require.NoError(t, err)
		assert.Equal(t, New(1250, "USD"), got)
	})

	t.Run("mismatch error names both currencies", func(t *testing.T) {
		t.Parallel()

		_, err := New(1000, "USD").Add(New(1000, "IDR"))
		require.EqualError(t, err, "currency mismatch: USD + IDR",
			"the error must say which pair collided, or the log is useless")
	})

	t.Run("mismatch yields the zero value, not a plausible amount", func(t *testing.T) {
		t.Parallel()

		got, err := New(1000, "USD").Add(New(1000, "IDR"))
		require.Error(t, err)
		assert.Equal(t, Money{}, got)
	})
}

func TestMulQty(t *testing.T) {
	t.Parallel()

	t.Run("scales by quantity", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, New(2997, "USD"), New(999, "USD").MulQty(3))
	})

	t.Run("quantity of zero yields zero in the same currency", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, New(0, "USD"), New(999, "USD").MulQty(0))
	})

	t.Run("quantity of one is identity", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, New(999, "IDR"), New(999, "IDR").MulQty(1))
	})

	t.Run("preserves currency", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "IDR", New(1500, "IDR").MulQty(4).Currency)
	})
}

func TestEqual(t *testing.T) {
	t.Parallel()

	t.Run("true when amount and currency both match", func(t *testing.T) {
		t.Parallel()

		assert.True(t, New(1000, "USD").Equal(New(1000, "USD")))
	})

	t.Run("false when amounts differ", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(1000, "USD").Equal(New(1001, "USD")))
	})

	t.Run("false when only the currency differs", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(1000, "USD").Equal(New(1000, "IDR")),
			"same amount in a different currency is not equal")
	})

	t.Run("false when both differ", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(1000, "USD").Equal(New(500, "IDR")))
	})

	t.Run("zero amounts in different currencies are not equal", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(0, "USD").Equal(New(0, "IDR")),
			"zero is still denominated; only IsZero ignores the currency")
	})
}

func TestIsZero(t *testing.T) {
	t.Parallel()

	t.Run("denominated zero is zero", func(t *testing.T) {
		t.Parallel()

		assert.True(t, New(0, "USD").IsZero())
	})

	t.Run("zero value of the struct is zero", func(t *testing.T) {
		t.Parallel()

		assert.True(t, Money{}.IsZero())
	})

	t.Run("non-zero amount is not zero", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(1, "USD").IsZero())
	})

	t.Run("negative amount is not zero", func(t *testing.T) {
		t.Parallel()

		assert.False(t, New(-1, "USD").IsZero())
	})
}
