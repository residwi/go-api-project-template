package testhelper

import (
	"context"
)

// FakeTxRunner satisfies the platform/database package's TxRunner interface
// for unit tests by running fn inline. A service under unit test needs
// atomicity to be a no-op, not a fake connection smuggled into the context.
//
// This package does not import platform/database to assert that here:
// platform/database's own in-package tests use testhelper (for
// MustStartPostgres), so an import back from here would cycle. The
// compile-time assertion instead lives in txrunner_test.go's external test
// package (testhelper_test), which can import platform/database freely.
type FakeTxRunner struct{}

func (FakeTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
