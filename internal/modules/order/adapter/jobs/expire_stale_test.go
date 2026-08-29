package jobs

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestExpireStaleWorkerRecoversThenExpires(t *testing.T) {
	t.Parallel()

	svc := NewMockStaleSweeper(t)
	svc.EXPECT().RecoverStale(mock.Anything).Return(nil)
	svc.EXPECT().ExpireStale(mock.Anything).Return(nil)

	w := NewExpireStaleWorker(svc, slog.New(slog.DiscardHandler), 2*time.Minute)
	err := w.Work(t.Context(), &river.Job[ExpireStaleArgs]{
		JobRow: &rivertype.JobRow{Kind: "order.expire-stale"},
		Args:   ExpireStaleArgs{},
	})

	require.NoError(t, err)
}

func TestExpireStaleWorkerExpiresEvenWhenRecoveryFails(t *testing.T) {
	t.Parallel()

	svc := NewMockStaleSweeper(t)
	svc.EXPECT().RecoverStale(mock.Anything).Return(errors.New("db down"))
	svc.EXPECT().ExpireStale(mock.Anything).Return(nil)

	w := NewExpireStaleWorker(svc, slog.New(slog.DiscardHandler), 2*time.Minute)

	require.NoError(t, w.Work(t.Context(), &river.Job[ExpireStaleArgs]{
		JobRow: &rivertype.JobRow{Kind: "order.expire-stale"},
		Args:   ExpireStaleArgs{},
	}))
}
