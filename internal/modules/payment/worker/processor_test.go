package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestProcessorSweep(t *testing.T) {
	t.Parallel()

	t.Run("runs recovery before expiry", func(t *testing.T) {
		t.Parallel()

		orders := NewMockOrderHousekeeper(t)
		var calls []string
		orders.EXPECT().RecoverStaleProcessing(context.Background()).
			RunAndReturn(func(context.Context) error {
				calls = append(calls, "recover")
				return nil
			})
		orders.EXPECT().ExpireStale(context.Background()).
			RunAndReturn(func(context.Context) error {
				calls = append(calls, "expire")
				return nil
			})

		p := NewProcessor(nil, orders, testhelper.DiscardLogger())

		require.NoError(t, p.Sweep(context.Background()))
		require.Equal(t, []string{"recover", "expire"}, calls)
	})

	t.Run("recovery failure is logged, not returned, and expiry still runs", func(t *testing.T) {
		t.Parallel()

		orders := NewMockOrderHousekeeper(t)
		orders.EXPECT().RecoverStaleProcessing(context.Background()).
			Return(errors.New("recover boom"))
		orders.EXPECT().ExpireStale(context.Background()).Return(nil)

		p := NewProcessor(nil, orders, testhelper.DiscardLogger())

		require.NoError(t, p.Sweep(context.Background()), "a recovery failure must not stop the tick")
	})

	t.Run("expiry failure is returned", func(t *testing.T) {
		t.Parallel()

		orders := NewMockOrderHousekeeper(t)
		orders.EXPECT().RecoverStaleProcessing(context.Background()).Return(nil)
		orders.EXPECT().ExpireStale(context.Background()).Return(errors.New("expire boom"))

		p := NewProcessor(nil, orders, testhelper.DiscardLogger())

		require.EqualError(t, p.Sweep(context.Background()), "expire boom")
	})
}
