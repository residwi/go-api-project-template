package testutil

import (
	"context"
	"sync"

	"github.com/residwi/go-api-project-template/internal/platform/jobs"
)

type FakeQueue struct {
	mu        sync.Mutex
	Inserted  []jobs.Record
	Cancelled []string
}

func (f *FakeQueue) Insert(_ context.Context, r jobs.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Inserted = append(f.Inserted, r)
	return nil
}

func (f *FakeQueue) CancelByGroupKey(_ context.Context, groupKey string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Cancelled = append(f.Cancelled, groupKey)
	return 1, nil
}
