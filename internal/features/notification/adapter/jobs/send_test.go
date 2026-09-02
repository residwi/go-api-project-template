package jobs

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSendWorkerCallsSend(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	svc := NewMockSender(t)
	svc.EXPECT().Send(mock.Anything, id).Return(nil)

	w := NewSendWorker(svc, 30*time.Second)
	err := w.Work(t.Context(), &river.Job[SendArgs]{
		JobRow: &rivertype.JobRow{Kind: "notification.send"},
		Args:   SendArgs{NotificationID: id},
	})

	require.NoError(t, err)
}
