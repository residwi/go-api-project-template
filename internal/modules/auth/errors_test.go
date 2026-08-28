package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestAuthSentinelsUnwrapToGenericKind(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		sentinel error
		kind     error
	}{
		"invalid credentials": {ErrInvalidCredentials, errs.ErrUnauthorized},
		"invalid token":       {ErrInvalidToken, errs.ErrUnauthorized},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, tc.sentinel, tc.kind)
		})
	}
}
