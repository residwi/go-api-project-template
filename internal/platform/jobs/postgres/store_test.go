package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testDB database.DB

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_platform_jobs")
	defer cleanup()
	testDB = database.DB{Primary: pool}
	os.Exit(m.Run())
}

func TestInsertAndClaim(t *testing.T) {
	setup(t)
	store := New(testDB)
	ctx := context.Background()

	t.Run("claims a pending record whose run_at has passed", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-claim-1", "")))

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)

		require.NoError(t, err)
		require.Len(t, claimed, 1)
		assert.Equal(t, "payment.refund", claimed[0].Kind)
		assert.Equal(t, "processing", claimed[0].Status)
		assert.NotNil(t, claimed[0].LockedUntil)
	})

	t.Run("does not claim a record on another queue", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("notification", "notification.send", "dedup-claim-2", "")))

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)

		require.NoError(t, err)
		for _, c := range claimed {
			assert.NotEqual(t, "notification.send", c.Kind)
		}
	})

	t.Run("does not claim a record scheduled for the future", func(t *testing.T) {
		future := rec("payment", "payment.refund", "dedup-claim-3", "")
		future.RunAt = time.Now().Add(time.Hour)
		require.NoError(t, store.Insert(ctx, future))

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)

		require.NoError(t, err)
		for _, c := range claimed {
			assert.NotEqual(t, "dedup-claim-3", c.DedupKey)
		}
	})

	t.Run("refuses a second active record with the same dedup key", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-unique", "")))

		err := store.Insert(ctx, rec("payment", "payment.refund", "dedup-unique", ""))

		require.Error(t, err)
	})

	t.Run("allows a new record once the previous one is completed", func(t *testing.T) {
		first := rec("payment", "payment.refund", "dedup-reuse", "")
		require.NoError(t, store.Insert(ctx, first))

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)
		require.NoError(t, err)
		var id uuid.UUID
		for _, c := range claimed {
			if c.DedupKey == "dedup-reuse" {
				id = c.ID
			}
		}
		require.NotEqual(t, uuid.Nil, id)
		require.NoError(t, store.Complete(ctx, id))

		assert.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-reuse", "")))
	})

	t.Run("allows many records to share a group key", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-g1", "order:group-a")))
		assert.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-g2", "order:group-a")))
	})
}

func TestClaimReclaimsExpiredLeases(t *testing.T) {
	setup(t)
	store := New(testDB)
	ctx := context.Background()

	t.Run("reclaims a processing record whose lease has lapsed", func(t *testing.T) {
		id := claimOne(t, store, "dedup-lapsed")
		expireLease(t, id)

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)

		require.NoError(t, err)
		require.Len(t, claimed, 1)
		assert.Equal(t, id, claimed[0].ID)
	})

	t.Run("leaves a processing record whose lease is still held", func(t *testing.T) {
		held := claimOne(t, store, "dedup-held")

		claimed, err := store.Claim(ctx, "payment", 10, time.Minute)

		require.NoError(t, err)
		for _, c := range claimed {
			assert.NotEqual(t, held, c.ID, "a live lease must not be reclaimed")
		}
	})
}

func TestCancelByGroupKey(t *testing.T) {
	setup(t)
	store := New(testDB)
	ctx := context.Background()

	t.Run("cancels every pending record in the group and nothing else", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-cg1", "order:cancel-me")))
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-cg2", "order:cancel-me")))
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-cg3", "order:keep-me")))

		n, err := store.CancelByGroupKey(ctx, "order:cancel-me")

		require.NoError(t, err)
		assert.Equal(t, 2, n)
	})

	t.Run("returns zero for a group with nothing pending", func(t *testing.T) {
		n, err := store.CancelByGroupKey(ctx, "order:nothing-here")

		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})
}

func TestSettle(t *testing.T) {
	setup(t)
	store := New(testDB)
	ctx := context.Background()

	t.Run("Retry returns the record to pending with a future run_at", func(t *testing.T) {
		id := claimOne(t, store, "dedup-retry")
		when := time.Now().Add(30 * time.Second)

		require.NoError(t, store.Retry(ctx, id, 1, "boom", when))

		got := fetch(t, id)
		assert.Equal(t, "pending", got.Status)
		assert.Equal(t, 1, got.Attempts)
		assert.Equal(t, "boom", got.LastError)
		assert.WithinDuration(t, when, got.RunAt, time.Second)
		assert.Nil(t, got.LockedUntil)
	})

	t.Run("Bury marks the record dead", func(t *testing.T) {
		id := claimOne(t, store, "dedup-bury")

		require.NoError(t, store.Bury(ctx, id, 3, "gave up"))

		got := fetch(t, id)
		assert.Equal(t, "dead", got.Status)
		assert.Equal(t, "gave up", got.LastError)
	})

	t.Run("Cancel marks the record cancelled", func(t *testing.T) {
		id := claimOne(t, store, "dedup-cancel")

		require.NoError(t, store.Cancel(ctx, id, "refused"))

		got := fetch(t, id)
		assert.Equal(t, "cancelled", got.Status)
	})
}

func TestPrune(t *testing.T) {
	setup(t)
	store := New(testDB)
	ctx := context.Background()

	t.Run("removes only finished records older than the age", func(t *testing.T) {
		id := claimOne(t, store, "dedup-prune")
		require.NoError(t, store.Complete(ctx, id))

		removed, err := store.Prune(ctx, "payment", 0, 100)

		require.NoError(t, err)
		assert.Equal(t, 1, removed)
	})

	t.Run("leaves pending records alone", func(t *testing.T) {
		require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", "dedup-prune-keep", "")))

		_, err := store.Prune(ctx, "payment", 0, 100)
		require.NoError(t, err)

		var count int
		err = testDB.Primary.QueryRow(ctx,
			`SELECT COUNT(*) FROM job_queue WHERE dedup_key = $1`, "dedup-prune-keep").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func rec(queue, kind, dedup, group string) jobs.Record {
	return jobs.Record{
		Queue:       queue,
		Kind:        kind,
		Payload:     []byte(`{"Amount":1}`),
		DedupKey:    dedup,
		GroupKey:    group,
		Status:      "pending",
		MaxAttempts: 3,
		RunAt:       time.Now().Add(-time.Minute),
	}
}

func claimOne(t *testing.T, store *Store, dedup string) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, store.Insert(ctx, rec("payment", "payment.refund", dedup, "")))

	claimed, err := store.Claim(ctx, "payment", 50, time.Minute)
	require.NoError(t, err)

	for _, c := range claimed {
		if c.DedupKey == dedup {
			return c.ID
		}
	}

	t.Fatalf("record %s was not claimed", dedup)
	return uuid.Nil
}

func fetch(t *testing.T, id uuid.UUID) jobs.Record {
	t.Helper()

	var r jobs.Record
	var lastError *string
	err := testDB.Primary.QueryRow(context.Background(),
		`SELECT id, queue, kind, payload, COALESCE(dedup_key, ''), COALESCE(group_key, ''),
		        status, attempts, max_attempts, last_error, locked_until, run_at
		 FROM job_queue WHERE id = $1`, id,
	).Scan(&r.ID, &r.Queue, &r.Kind, &r.Payload, &r.DedupKey, &r.GroupKey,
		&r.Status, &r.Attempts, &r.MaxAttempts, &lastError, &r.LockedUntil, &r.RunAt)
	require.NoError(t, err)
	if lastError != nil {
		r.LastError = *lastError
	}
	return r
}

func expireLease(t *testing.T, id uuid.UUID) {
	t.Helper()

	_, err := testDB.Primary.Exec(context.Background(),
		`UPDATE job_queue SET locked_until = NOW() - INTERVAL '1 minute' WHERE id = $1`, id)
	require.NoError(t, err)
}

func setup(t *testing.T) {
	t.Helper()
	testutil.ResetDB(t, testDB.Primary)
}
