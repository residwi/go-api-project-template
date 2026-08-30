package payment

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestPaymentSentinelsUnwrapToGenericKind(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, ErrNotRefundable, errs.ErrConflict)
}
