package payment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/features/payment"
)

func TestNewWorker(t *testing.T) {
	t.Run("constructs worker with all dependencies", func(t *testing.T) {
		w := payment.NewWorker(nil, nil, nil, nil, nil, nil, nil, nil, payment.WorkerConfig{})
		assert.NotNil(t, w)
	})
}
