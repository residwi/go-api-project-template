package jobs_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/jobs"
)

func testConfig() jobs.Config {
	return jobs.Config{
		Interval:      time.Second,
		BatchSize:     10,
		LeaseDuration: time.Minute,
		Concurrency:   4,
		PruneAge:      24 * time.Hour,
		PruneLimit:    100,
	}
}

func TestRunner(t *testing.T) {
	t.Run("processes every claimed job", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			q := &fakeQueue{batches: [][]testJob{{{ID: 1}, {ID: 2}}}}
			p := &fakeProcessor{}
			r := jobs.NewRunner("test", q, p, testConfig())

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond) // advance past the first tick
			synctest.Wait()                     // let the tick's processing drain

			assert.ElementsMatch(t, []testJob{{ID: 1}, {ID: 2}}, p.snapshot())

			cancel()
			synctest.Wait() // let the runner observe cancellation and return
		})
	})

	t.Run("prunes each tick", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			q := &fakeQueue{}
			p := &fakeProcessor{}
			r := jobs.NewRunner("test", q, p, testConfig())

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond) // one tick
			synctest.Wait()

			q.mu.Lock()
			calls := q.pruneCalls
			q.mu.Unlock()
			assert.Equal(t, 1, calls)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("sweeps when the processor implements Sweeper", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			q := &fakeQueue{}
			p := &fakeSweepProcessor{fakeProcessor: &fakeProcessor{}}
			r := jobs.NewRunner("test", q, p, testConfig())

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond) // one tick
			synctest.Wait()

			p.sweepMu.Lock()
			calls := p.sweepCalls
			p.sweepMu.Unlock()
			assert.Equal(t, 1, calls)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("bounds each job's context to a fraction of the lease", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			var (
				captureMu   sync.Mutex
				captured    bool
				hasDeadline bool
				remaining   time.Duration
			)
			p := &fakeProcessor{fn: func(jobCtx context.Context, _ testJob) error {
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
			}}
			q := &fakeQueue{batches: [][]testJob{{{ID: 1}}}}
			r := jobs.NewRunner("test", q, p, testConfig())

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond) // one tick
			synctest.Wait()

			captureMu.Lock()
			hd, rem := hasDeadline, remaining
			captureMu.Unlock()
			assert.True(t, hd, "job context should carry a deadline")
			// leaseSafetyDivisor = 5 → timeout is 4/5 of the 1m lease.
			assert.Equal(t, 48*time.Second, rem)

			cancel()
			synctest.Wait()
		})
	})

	t.Run("never runs more than Concurrency jobs at once", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			release := make(chan struct{})
			var (
				mu          sync.Mutex
				inFlight    int
				maxInFlight int
			)
			p := &fakeProcessor{fn: func(_ context.Context, _ testJob) error {
				mu.Lock()
				inFlight++
				if inFlight > maxInFlight {
					maxInFlight = inFlight
				}
				mu.Unlock()

				<-release // hold the slot until the test releases everyone

				mu.Lock()
				inFlight--
				mu.Unlock()
				return nil
			}}

			cfg := testConfig()
			cfg.Concurrency = 2
			q := &fakeQueue{batches: [][]testJob{{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}}}
			r := jobs.NewRunner("test", q, p, cfg)

			go r.Start(ctx)

			time.Sleep(1500 * time.Millisecond) // one tick claims all four
			synctest.Wait()                     // settles with the cap's worth in-flight, the rest queued

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

type testJob struct{ ID int }

type fakeQueue struct {
	mu         sync.Mutex
	batches    [][]testJob
	claimErr   error
	claimCalls int
	pruneCalls int
}

func (q *fakeQueue) Claim(_ context.Context, _ int, _ time.Duration) ([]testJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claimCalls++
	if q.claimErr != nil {
		return nil, q.claimErr
	}
	if len(q.batches) == 0 {
		return nil, nil
	}
	batch := q.batches[0]
	q.batches = q.batches[1:]
	return batch, nil
}

func (q *fakeQueue) Prune(_ context.Context, _ time.Duration, _ int) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pruneCalls++
	return 0, nil
}

type fakeProcessor struct {
	mu        sync.Mutex
	processed []testJob
	fn        func(context.Context, testJob) error
}

func (p *fakeProcessor) Process(ctx context.Context, job testJob) error {
	p.mu.Lock()
	p.processed = append(p.processed, job)
	fn := p.fn
	p.mu.Unlock()
	if fn != nil {
		return fn(ctx, job)
	}
	return nil
}

func (p *fakeProcessor) snapshot() []testJob {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]testJob(nil), p.processed...)
}

type fakeSweepProcessor struct {
	*fakeProcessor

	sweepMu    sync.Mutex
	sweepCalls int
}

func (p *fakeSweepProcessor) Sweep(_ context.Context) error {
	p.sweepMu.Lock()
	defer p.sweepMu.Unlock()
	p.sweepCalls++
	return nil
}
