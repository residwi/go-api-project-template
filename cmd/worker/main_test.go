package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	paymentdomain "github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// These three subtests are payment/worker/processor_test.go's TestProcessorSweep,
// ported verbatim (git show 5749011:internal/modules/payment/worker/processor_test.go):
// that file died with the package it tested, and it is the one thing that would
// have caught the brief's backwards, always-swallow Sweep snippet.
func TestPaymentProcessor_Sweep(t *testing.T) {
	t.Parallel()

	t.Run("runs recovery before expiry", func(t *testing.T) {
		t.Parallel()

		var calls []string
		p := paymentProcessor{
			recover: func(context.Context) error {
				calls = append(calls, "recover")
				return nil
			},
			expire: func(context.Context) error {
				calls = append(calls, "expire")
				return nil
			},
			logger: testutil.DiscardLogger(),
		}

		require.NoError(t, p.Sweep(context.Background()))
		require.Equal(t, []string{"recover", "expire"}, calls)
	})

	t.Run("recovery failure is logged, not returned, and expiry still runs", func(t *testing.T) {
		t.Parallel()

		var expireRan bool
		p := paymentProcessor{
			recover: func(context.Context) error { return errors.New("recover boom") },
			expire: func(context.Context) error {
				expireRan = true
				return nil
			},
			logger: testutil.DiscardLogger(),
		}

		require.NoError(t, p.Sweep(context.Background()), "a recovery failure must not stop the tick")
		require.True(t, expireRan)
	})

	t.Run("expiry failure is returned", func(t *testing.T) {
		t.Parallel()

		p := paymentProcessor{
			recover: func(context.Context) error { return nil },
			expire:  func(context.Context) error { return errors.New("expire boom") },
			logger:  testutil.DiscardLogger(),
		}

		require.EqualError(t, p.Sweep(context.Background()), "expire boom")
	})

	// internal/platform/jobs/runner_test.go:64 covers the runner's own half of this
	// with a POINTER-receiver fakeSweepProcessor passed as &fakeSweepProcessor{} --
	// the opposite convention from paymentProcessor's value receivers. That test
	// alone cannot catch paymentProcessor switching to pointer receivers while
	// cmd/worker still passes it by value: the generic runner test never touches
	// this concrete type, so the sweep would silently stop with no failing test
	// and no log line. This subtest is what closes that gap.
	t.Run("reaches jobs.Runner as a Sweeper", func(t *testing.T) {
		t.Parallel()

		var p jobs.LegacyProcessor[paymentdomain.Job] = paymentProcessor{}
		_, ok := p.(jobs.Sweeper)
		require.True(t, ok, "runner.go:74 asserts this; a pointer receiver here silently kills the sweep")
	})
}
