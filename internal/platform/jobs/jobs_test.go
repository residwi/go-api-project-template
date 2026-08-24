package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEnqueue(t *testing.T) {
	t.Parallel()

	t.Run("marshals exported fields and derives the queue from the kind", func(t *testing.T) {
		t.Parallel()

		e := NewMockEnqueuer(t)
		var got Record
		e.EXPECT().Insert(mock.Anything, mock.Anything).Run(func(_ context.Context, r Record) {
			got = r
		}).Return(nil)

		err := Enqueue(context.Background(), e, probeJob{Amount: 500}, Keys{
			Dedup: "test.thing:1",
			Group: "order:9",
		})

		require.NoError(t, err)
		assert.Equal(t, "test", got.Queue)
		assert.Equal(t, "test.thing.do", got.Kind)
		assert.Equal(t, "test.thing:1", got.DedupKey)
		assert.Equal(t, "order:9", got.GroupKey)
		assert.Equal(t, "pending", got.Status)
		assert.Equal(t, 3, got.MaxAttempts)
		assert.JSONEq(t, `{"Amount":500}`, string(got.Payload))
	})

	t.Run("never marshals an unexported dependency", func(t *testing.T) {
		t.Parallel()

		e := NewMockEnqueuer(t)
		var got Record
		e.EXPECT().Insert(mock.Anything, mock.Anything).Run(func(_ context.Context, r Record) {
			got = r
		}).Return(nil)

		err := Enqueue(context.Background(), e, probeJob{Amount: 1, dep: &fakeDep{}}, Keys{})

		require.NoError(t, err)
		assert.JSONEq(t, `{"Amount":1}`, string(got.Payload))
	})

	t.Run("MaxAttempts overrides the default", func(t *testing.T) {
		t.Parallel()

		e := NewMockEnqueuer(t)
		var got Record
		e.EXPECT().Insert(mock.Anything, mock.Anything).Run(func(_ context.Context, r Record) {
			got = r
		}).Return(nil)

		err := Enqueue(context.Background(), e, probeJob{}, Keys{}, MaxAttempts(7))

		require.NoError(t, err)
		assert.Equal(t, 7, got.MaxAttempts)
	})

	t.Run("RunAt sets a future schedule", func(t *testing.T) {
		t.Parallel()

		e := NewMockEnqueuer(t)
		var got Record
		e.EXPECT().Insert(mock.Anything, mock.Anything).Run(func(_ context.Context, r Record) {
			got = r
		}).Return(nil)
		when := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

		err := Enqueue(context.Background(), e, probeJob{}, Keys{}, RunAt(when))

		require.NoError(t, err)
		assert.Equal(t, when, got.RunAt)
	})

	t.Run("queue is the kind's first segment", func(t *testing.T) {
		t.Parallel()

		cases := map[string]string{
			"payment.refund":    "payment",
			"notification.send": "notification",
			"test.thing.do":     "test",
			"bare":              "bare",
		}
		for kind, want := range cases {
			assert.Equal(t, want, queueOf(kind), "kind %q", kind)
		}
	})
}

func TestRegistry(t *testing.T) {
	t.Parallel()

	t.Run("routes a record to the handler registered for its kind", func(t *testing.T) {
		t.Parallel()

		dep := &fakeDep{}
		reg := NewRegistry()
		Register(reg, probeJob{dep: dep})

		err := reg.Process(context.Background(), Record{
			Kind:    "test.thing.do",
			Payload: []byte(`{"Amount":42}`),
		})

		require.NoError(t, err)
		assert.Equal(t, []int{42}, dep.seen)
	})

	t.Run("keeps the prototype dependency and gives each job its own payload", func(t *testing.T) {
		t.Parallel()

		dep := &fakeDep{}
		reg := NewRegistry()
		Register(reg, probeJob{dep: dep})

		require.NoError(t, reg.Process(context.Background(), Record{
			Kind: "test.thing.do", Payload: []byte(`{"Amount":1}`),
		}))
		require.NoError(t, reg.Process(context.Background(), Record{
			Kind: "test.thing.do", Payload: []byte(`{"Amount":2}`),
		}))

		assert.Equal(t, []int{1, 2}, dep.seen)
	})

	t.Run("returns an error for an unregistered kind", func(t *testing.T) {
		t.Parallel()

		reg := NewRegistry()

		err := reg.Process(context.Background(), Record{Kind: "nobody.home"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "nobody.home")
	})

	t.Run("propagates ErrDiscard from the handler", func(t *testing.T) {
		t.Parallel()

		reg := NewRegistry()
		Register(reg, probeJob{dep: &fakeDep{discard: true}})

		err := reg.Process(context.Background(), Record{
			Kind: "test.thing.do", Payload: []byte(`{"Amount":1}`),
		})

		require.ErrorIs(t, err, ErrDiscard)
	})
}

type fakeDep struct {
	seen    []int
	discard bool
}

type probeJob struct {
	Amount int
	dep    *fakeDep
}

func (probeJob) Kind() string { return "test.thing.do" }

func (j probeJob) Run(_ context.Context) error {
	if j.dep.discard {
		return fmt.Errorf("refused: %w", ErrDiscard)
	}
	j.dep.seen = append(j.dep.seen, j.Amount)
	return nil
}
