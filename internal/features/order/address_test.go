package order_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/order"
)

func TestAddress_JSONRoundTrip(t *testing.T) {
	addr := order.Address{
		Street:  "123 Main St",
		City:    "Springfield",
		State:   "IL",
		ZipCode: "62701",
		Country: "US",
	}

	data, err := json.Marshal(addr)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"street":"123 Main St",
		"city":"Springfield",
		"state":"IL",
		"zip_code":"62701",
		"country":"US"
	}`, string(data))

	var decoded order.Address
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, addr, decoded)
}

func TestAddress_ZeroValue(t *testing.T) {
	var addr order.Address
	assert.Equal(t, order.Address{}, addr)
}
