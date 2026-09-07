package errs_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestKind(t *testing.T) {
	t.Parallel()

	t.Run("matches a bare sentinel", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, errs.ErrNotFound, errs.Kind(errs.ErrNotFound))
	})

	t.Run("matches a wrapped sentinel", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, errs.ErrConflict, errs.Kind(fmt.Errorf("%w: cart is empty", errs.ErrConflict)))
	})

	t.Run("returns nil for an unrelated error", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, errs.Kind(errors.New("connection refused")))
	})

	t.Run("returns nil for nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, errs.Kind(nil))
	})
}
