package testhelper_test

import (
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// Here rather than in txrunner.go because package testhelper cannot import
// platform/database without cycling -- its tests import testhelper. An external
// test file compiles as its own package, so it can.
var _ database.TxRunner = testhelper.FakeTxRunner{}
