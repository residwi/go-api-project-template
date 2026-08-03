package order_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/modules/order"
)

// Address's JSON shape is no longer this package's concern -- it carries no
// json tags (order/http/handler.go's addressResponse and addressRequest own the
// wire mapping now, and order/http/handler_test.go pins the round trip). This
// file keeps only the plain domain-level assertion.
func TestAddress_ZeroValue(t *testing.T) {
	var addr order.Address
	assert.Equal(t, order.Address{}, addr)
}
