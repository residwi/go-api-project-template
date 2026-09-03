package identity_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
)

func TestFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns what NewContext stored", func(t *testing.T) {
		t.Parallel()

		want := identity.Identity{UserID: uuid.New(), Role: "admin"}

		got, ok := identity.FromContext(identity.NewContext(context.Background(), want))

		assert.True(t, ok)
		assert.Equal(t, want, got)
	})

	t.Run("reports absence rather than a zero identity", func(t *testing.T) {
		t.Parallel()

		got, ok := identity.FromContext(context.Background())

		assert.False(t, ok)
		assert.Equal(t, identity.Identity{}, got)
	})

	// The key is an unexported struct type, so nothing outside this package can
	// forge an identity by writing the same context key.
	t.Run("ignores a value stored under a lookalike key", func(t *testing.T) {
		t.Parallel()

		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, identity.Identity{Role: "admin"})

		got, ok := identity.FromContext(ctx)

		assert.False(t, ok)
		assert.Equal(t, identity.Identity{}, got)
	})
}
