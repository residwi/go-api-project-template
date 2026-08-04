package testhelper_test

import (
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// Compile-time assertion that FakeTxRunner satisfies database.TxRunner. This
// lives in the external test package (testhelper_test) rather than in
// txrunner.go itself: package testhelper cannot import platform/database
// without cycling back through it (platform/database's own in-package tests
// import testhelper for MustStartPostgres), but an external test file can
// import database freely since it compiles as its own package.
var _ database.TxRunner = testhelper.FakeTxRunner{}
