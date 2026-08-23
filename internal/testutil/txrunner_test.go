package testutil_test

import (
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// Here rather than in txrunner.go because package testutil cannot import
// platform/database without cycling -- its tests import testutil. An external
// test file compiles as its own package, so it can.
var _ database.TxRunner = testutil.FakeTxRunner{}
