package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("writes the row then enqueues a send job keyed on its id", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		userID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, n *domain.Notification) error {
				n.ID = id
				return nil
			})

		queue := &testutil.FakeQueue{}
		svc := New(repo, testutil.FakeTxRunner{}, queue, NewMockChannel(t), testutil.DiscardLogger())

		err := svc.Create(context.Background(), NewNotification{
			UserID: userID,
			Title:  "Order Placed",
			Body:   "Your order has been placed.",
		})

		require.NoError(t, err)
		require.Len(t, queue.Inserted, 1)
		assert.Equal(t, "notification.send", queue.Inserted[0].Kind)
		assert.Equal(t, "notification", queue.Inserted[0].Queue)
		assert.Equal(t, "notification.send:"+id.String(), queue.Inserted[0].DedupKey)
		assert.Empty(t, queue.Inserted[0].GroupKey)
	})

	t.Run("does not enqueue when the row fails to write", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("boom"))

		queue := &testutil.FakeQueue{}
		svc := New(repo, testutil.FakeTxRunner{}, queue, NewMockChannel(t), testutil.DiscardLogger())

		err := svc.Create(context.Background(), NewNotification{UserID: uuid.New(), Title: "t"})

		require.Error(t, err)
		assert.Empty(t, queue.Inserted)
	})
}

func TestSendJob(t *testing.T) {
	t.Parallel()

	t.Run("reads the row and hands it to the channel", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		n := domain.Notification{ID: id, UserID: uuid.New(), Title: "Order Placed"}

		repo := NewMockRepository(t)
		repo.EXPECT().Get(mock.Anything, id).Return(n, nil)

		ch := NewMockChannel(t)
		ch.EXPECT().Send(mock.Anything, n).Return(nil)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, ch, testutil.DiscardLogger())

		err := NewSendJob(svc).withID(id).Run(context.Background())

		require.NoError(t, err)
	})

	t.Run("marshals only the notification id", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()

		out, err := json.Marshal(SendJob{NotificationID: id, svc: &Service{}})

		require.NoError(t, err)
		assert.JSONEq(t, `{"NotificationID":"`+id.String()+`"}`, string(out))
	})
}

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Notification{
			{ID: uuid.New(), UserID: userID, Title: "Order Placed"},
			{ID: uuid.New(), UserID: userID, Title: "Order Shipped"},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		result, err := svc.List(t.Context(), userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		_, err := svc.List(t.Context(), userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_CountUnread(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(5, nil)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		count, err := svc.CountUnread(t.Context(), userID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(0, assert.AnError)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		_, err := svc.CountUnread(t.Context(), userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_MarkRead(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID, id := uuid.New(), uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(nil)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		err := svc.MarkRead(t.Context(), userID, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID, id := uuid.New(), uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(errs.ErrNotFound)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		err := svc.MarkRead(t.Context(), userID, id)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_MarkAllRead(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(nil)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		err := svc.MarkAllRead(t.Context(), userID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(assert.AnError)

		svc := New(repo, testutil.FakeTxRunner{}, &testutil.FakeQueue{}, NewMockChannel(t), testutil.DiscardLogger())
		err := svc.MarkAllRead(t.Context(), userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func (j SendJob) withID(id uuid.UUID) SendJob {
	j.NotificationID = id
	return j
}
