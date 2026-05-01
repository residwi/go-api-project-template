package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
)

func TestMoney_JSONRoundTrip(t *testing.T) {
	m := core.Money{Amount: 1999, Currency: "USD"}

	data, err := json.Marshal(m)
	require.NoError(t, err)
	assert.JSONEq(t, `{"amount":1999,"currency":"USD"}`, string(data))

	var decoded core.Money
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, m, decoded)
}

func TestMoney_ZeroValue(t *testing.T) {
	var m core.Money
	assert.Equal(t, core.Money{}, m)
}
