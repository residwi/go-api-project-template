// Package empty is cart's clear-the-cart slice. Named empty, not clear:
// clear is a Go 1.21+ predeclared identifier (the builtin clearing a map or
// slice), and golangci-lint's predeclared linter rejects "package clear"
// exactly as it rejects "package delete" -- the same constraint that made
// cart's own item-delete slice "remove" instead. The exported Clear method
// below is unaffected: only a package name can shadow a predeclared
// identifier, not a method name.
package empty

import (
	"context"

	"github.com/google/uuid"
)

// Repository is empty's own storage. Its only implementation is
// empty/postgres, constructed in cart/module.go.
type Repository interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}
