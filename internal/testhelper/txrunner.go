package testhelper

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// FakeTxRunner satisfies database.TxRunner for unit tests by running fn inline.
// A service under unit test needs atomicity to be a no-op, not a fake connection
// smuggled into the context.
type FakeTxRunner struct{}

func (FakeTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

var _ database.TxRunner = FakeTxRunner{}
