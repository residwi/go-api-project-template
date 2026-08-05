package testhelper

import (
	"context"
)

// FakeTxRunner runs fn inline: a service under unit test needs atomicity to be a
// no-op, not a fake connection smuggled into the context.
//
// The compile-time assertion that it satisfies database.TxRunner lives in
// txrunner_test.go's external package, because importing platform/database here
// would cycle -- its own tests use testhelper.
type FakeTxRunner struct{}

func (FakeTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
