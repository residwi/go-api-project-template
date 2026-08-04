package testhelper

import (
	"context"
)

// FakeTxRunner satisfies the platform/database package's TxRunner interface
// for unit tests by running fn inline. A service under unit test needs
// atomicity to be a no-op, not a fake connection smuggled into the context.
//
// It does not import that package to assert this: platform/database's own
// in-package tests use testhelper (for MustStartPostgres), so an import back
// from here would cycle. Every NewService(..., FakeTxRunner{}) call site
// across the modules already enforces the interface match structurally.
type FakeTxRunner struct{}

func (FakeTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
