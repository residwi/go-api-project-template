package apperror_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestBusinessSentinelsUnwrapToGenericKind(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		sentinel error
		kind     error
	}{
		"insufficient stock": {apperror.ErrInsufficientStock, errs.ErrConflict},
		"cart empty":         {apperror.ErrCartEmpty, errs.ErrBadRequest},
		"order not payable":  {apperror.ErrOrderNotPayable, errs.ErrBadRequest},
		"order charging":     {apperror.ErrOrderCharging, errs.ErrConflict},
		"amount mismatch":    {apperror.ErrAmountMismatch, errs.ErrConflict},
		"coupon exhausted":   {apperror.ErrCouponExhausted, errs.ErrConflict},
		"already finalized":  {apperror.ErrAlreadyFinalized, errs.ErrConflict},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, tc.sentinel, tc.kind)
		})
	}
}

func TestWrappedBusinessSentinelStillMatchesKind(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("reserving stock: %w", apperror.ErrInsufficientStock)

	require.ErrorIs(t, err, apperror.ErrInsufficientStock)
	assert.ErrorIs(t, err, errs.ErrConflict)
}
