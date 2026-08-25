package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner(t *testing.T) {
	t.Parallel()

	t.Run("processes every claimed job", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			recs := []Record{rec("j1"), rec("j2")}
			store := newFakeStore(recs...)

			var mu sync.Mutex
			var processed []uuid.UUID
			proc := processorFunc(func(_ context.Context, r Record) error {
				mu.Lock()
				processed = append(processed, r.ID)
				mu.Unlock()
				return nil
			})
			r := NewRunner("payment", store, proc, testConfig(), slog.New(slog.DiscardHandler))

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			mu.Lock()
			got := append([]uuid.UUID(nil), processed...)
			mu.Unlock()
			assert.ElementsMatch(t, []uuid.UUID{recs[0].ID, recs[1].ID}, got)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("prunes each tick", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			store := newFakeStore()
			proc := processorFunc(func(context.Context, Record) error { return nil })
			r := NewRunner("payment", store, proc, testConfig(), slog.New(slog.DiscardHandler))

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			store.mu.Lock()
			calls := store.pruneCalls
			store.mu.Unlock()
			assert.Equal(t, 1, calls)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("sweeps when the processor implements Sweeper", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			store := newFakeStore()
			proc := &fakeSweepProcessor{processorFunc: func(context.Context, Record) error { return nil }}
			r := NewRunner("payment", store, proc, testConfig(), slog.New(slog.DiscardHandler))

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			proc.sweepMu.Lock()
			calls := proc.sweepCalls
			proc.sweepMu.Unlock()
			assert.Equal(t, 1, calls)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("bounds each job's context to a fraction of the lease", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			var (
				captureMu   sync.Mutex
				captured    bool
				hasDeadline bool
				remaining   time.Duration
			)
			proc := processorFunc(func(jobCtx context.Context, _ Record) error {
				captureMu.Lock()
				defer captureMu.Unlock()
				if captured {
					return nil
				}
				captured = true
				deadline, ok := jobCtx.Deadline()
				hasDeadline = ok
				if ok {
					remaining = time.Until(deadline)
				}
				return nil
			})
			store := newFakeStore(rec("j1"))
			r := NewRunner("payment", store, proc, testConfig(), slog.New(slog.DiscardHandler))

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			captureMu.Lock()
			hd, rem := hasDeadline, remaining
			captureMu.Unlock()
			assert.True(t, hd, "job context should carry a deadline")
			assert.Equal(t, 48*time.Second, rem)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("never runs more than Concurrency jobs at once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			release := make(chan struct{})
			var (
				mu          sync.Mutex
				inFlight    int
				maxInFlight int
			)
			proc := processorFunc(func(_ context.Context, _ Record) error {
				mu.Lock()
				inFlight++
				if inFlight > maxInFlight {
					maxInFlight = inFlight
				}
				mu.Unlock()

				<-release

				mu.Lock()
				inFlight--
				mu.Unlock()
				return nil
			})

			cfg := testConfig()
			cfg.Concurrency = 2
			store := newFakeStore(rec("j1"), rec("j2"), rec("j3"), rec("j4"))
			r := NewRunner("payment", store, proc, cfg, slog.New(slog.DiscardHandler))

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			mu.Lock()
			peak := maxInFlight
			mu.Unlock()
			assert.Equal(t, 2, peak)

			close(release)
			synctest.Wait()

			cancel()
			synctest.Wait()
		})
	})
}

func TestRunnerSettlesJobs(t *testing.T) {
	t.Parallel()

	t.Run("completes a job whose handler returns nil", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore(rec("j1"))
		runner := NewRunner("payment", store, processorFunc(func(context.Context, Record) error {
			return nil
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		assert.Equal(t, []string{"complete:j1"}, store.calls)
	})

	t.Run("retries a job whose handler returns a plain error", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore(rec("j1"))
		runner := NewRunner("payment", store, processorFunc(func(context.Context, Record) error {
			return errors.New("boom")
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		assert.Equal(t, []string{"retry:j1:1:boom"}, store.calls)
	})

	t.Run("buries a job whose retries are exhausted", func(t *testing.T) {
		t.Parallel()

		last := rec("j1")
		last.Attempts = 2
		last.MaxAttempts = 3

		store := newFakeStore(last)
		runner := NewRunner("payment", store, processorFunc(func(context.Context, Record) error {
			return errors.New("boom")
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		assert.Equal(t, []string{"bury:j1:3:boom"}, store.calls)
	})

	t.Run("cancels a job whose handler wraps ErrDiscard", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore(rec("j1"))
		runner := NewRunner("payment", store, processorFunc(func(context.Context, Record) error {
			return fmt.Errorf("not refundable: %w", ErrDiscard)
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		assert.Equal(t, []string{"cancel:j1:not refundable: discard job"}, store.calls)
	})

	t.Run("retries a job whose handler panicked", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore(rec("j1"))
		runner := NewRunner("payment", store, processorFunc(func(context.Context, Record) error {
			panic("kaboom")
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		require.Len(t, store.calls, 1)
		assert.Contains(t, store.calls[0], "retry:j1:1:")
		assert.Contains(t, store.calls[0], "kaboom")
	})

	t.Run("claims only its own queue", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore()
		runner := NewRunner("notification", store, processorFunc(func(context.Context, Record) error {
			return nil
		}), fastConfig(), slog.New(slog.DiscardHandler))

		runner.tick(context.Background())

		assert.Equal(t, []string{"notification"}, store.claimedQueues)
	})
}

func TestBackoff(t *testing.T) {
	t.Parallel()

	t.Run("grows exponentially and stays within the jitter band", func(t *testing.T) {
		t.Parallel()

		for attempts, base := range map[int]time.Duration{1: 2 * time.Second, 3: 8 * time.Second} {
			got := backoff(attempts)
			assert.GreaterOrEqual(t, got, base)
			assert.Less(t, got, base+base/2)
		}
	})

	t.Run("does not overflow at a high attempt count", func(t *testing.T) {
		t.Parallel()

		assert.Positive(t, backoff(1_000))
	})
}

type processorFunc func(context.Context, Record) error

func (f processorFunc) Process(ctx context.Context, rec Record) error { return f(ctx, rec) }

type fakeSweepProcessor struct {
	processorFunc

	sweepMu    sync.Mutex
	sweepCalls int
}

func (p *fakeSweepProcessor) Sweep(context.Context) error {
	p.sweepMu.Lock()
	defer p.sweepMu.Unlock()
	p.sweepCalls++
	return nil
}

type fakeStore struct {
	mu            sync.Mutex
	pending       []Record
	calls         []string
	claimedQueues []string
	pruneCalls    int
}

func newFakeStore(records ...Record) *fakeStore {
	return &fakeStore{pending: records}
}

func (f *fakeStore) Insert(context.Context, Record) error { return nil }

func (f *fakeStore) Claim(_ context.Context, queue string, _ int, _ time.Duration) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.claimedQueues = append(f.claimedQueues, queue)
	out := f.pending
	f.pending = nil
	return out, nil
}

func (f *fakeStore) Complete(_ context.Context, id uuid.UUID) error {
	return f.record(fmt.Sprintf("complete:%s", label(id)))
}

func (f *fakeStore) Retry(_ context.Context, id uuid.UUID, attempts int, lastErr string, _ time.Time) error {
	return f.record(fmt.Sprintf("retry:%s:%d:%s", label(id), attempts, lastErr))
}

func (f *fakeStore) Bury(_ context.Context, id uuid.UUID, attempts int, lastErr string) error {
	return f.record(fmt.Sprintf("bury:%s:%d:%s", label(id), attempts, lastErr))
}

func (f *fakeStore) Cancel(_ context.Context, id uuid.UUID, lastErr string) error {
	return f.record(fmt.Sprintf("cancel:%s:%s", label(id), lastErr))
}

func (f *fakeStore) CancelByGroupKey(context.Context, string) (int, error) { return 0, nil }

func (f *fakeStore) Prune(context.Context, string, time.Duration, int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pruneCalls++
	return 0, nil
}

func (f *fakeStore) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, call)
	return nil
}

func testConfig() Config {
	return Config{
		Interval:      time.Second,
		BatchSize:     10,
		LeaseDuration: time.Minute,
		Concurrency:   4,
		PruneAge:      24 * time.Hour,
		PruneLimit:    100,
	}
}

var (
	idLabels   = map[uuid.UUID]string{}
	idLabelsMu sync.Mutex
)

func rec(name string) Record {
	id := uuid.New()

	idLabelsMu.Lock()
	idLabels[id] = name
	idLabelsMu.Unlock()

	return Record{
		ID:          id,
		Queue:       "payment",
		Kind:        "payment.refund",
		Payload:     []byte(`{}`),
		Status:      "processing",
		MaxAttempts: 3,
		RunAt:       time.Now(),
	}
}

func label(id uuid.UUID) string {
	idLabelsMu.Lock()
	defer idLabelsMu.Unlock()

	return idLabels[id]
}

func fastConfig() Config {
	return Config{
		Interval:      time.Millisecond,
		BatchSize:     10,
		LeaseDuration: time.Minute,
		Concurrency:   2,
		PruneAge:      time.Hour,
		PruneLimit:    10,
	}
}
