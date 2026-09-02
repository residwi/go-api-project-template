package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestValidateParentSelf(t *testing.T) {
	t.Parallel()

	t.Run("rejects a category set as its own parent", func(t *testing.T) {
		t.Parallel()

		selfID := uuid.New()

		err := ValidateParentSelf(selfID, selfID)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "cannot be its own parent")
	})

	t.Run("a fresh category with no self id is never its own parent", func(t *testing.T) {
		t.Parallel()

		// selfID is uuid.Nil for a category that doesn't exist yet (create's
		// case), so parentID == selfID must not fire the self-parent guard.
		parentID := uuid.New()

		err := ValidateParentSelf(parentID, uuid.Nil)

		require.NoError(t, err)
	})

	t.Run("accepts distinct parent and self", func(t *testing.T) {
		t.Parallel()

		err := ValidateParentSelf(uuid.New(), uuid.New())

		require.NoError(t, err)
	})
}

func TestValidateParentDepth(t *testing.T) {
	t.Parallel()

	t.Run("rejects a parent that does not exist", func(t *testing.T) {
		t.Parallel()

		err := ValidateParentDepth(0, false)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "parent category not found")
	})

	t.Run("rejects a cycle", func(t *testing.T) {
		t.Parallel()

		err := ValidateParentDepth(2, true)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "circular parent reference")
	})

	t.Run("rejects a chain deeper than MaxDepth", func(t *testing.T) {
		t.Parallel()

		err := ValidateParentDepth(MaxDepth, false)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "depth exceeds maximum of 5")
	})

	t.Run("accepts a valid parent within depth", func(t *testing.T) {
		t.Parallel()

		err := ValidateParentDepth(1, false)

		require.NoError(t, err)
	})
}
