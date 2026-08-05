package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddress_ZeroValue(t *testing.T) {
	t.Parallel()

	var addr Address
	assert.Equal(t, Address{}, addr)
}
