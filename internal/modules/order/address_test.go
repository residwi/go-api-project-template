package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Address's JSON shape is no longer this package's concern -- it carries no
// json tags (order/http/handler.go's addressResponse and addressRequest own the
// wire mapping now, and order/http/handler_test.go pins the round trip). This
// file keeps only the plain domain-level assertion.
func TestAddress_ZeroValue(t *testing.T) {
	t.Parallel()

	var addr Address
	assert.Equal(t, Address{}, addr)
}
