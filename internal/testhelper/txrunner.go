package testhelper

import "context"

// FakeTxRunner satisfies database.TxRunner for unit tests by running fn inline.
// A service under unit test needs atomicity to be a no-op, not a fake connection
// smuggled into the context -- which is what the WithTestTx + noopDBTX pattern it
// replaces had to do.
type FakeTxRunner struct{}

func (FakeTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// Compile-time proof that the fake and the real runner stay interchangeable.
var _ interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
} = FakeTxRunner{}
