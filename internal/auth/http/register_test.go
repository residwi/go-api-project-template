package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/auth"
)

// TestToTokenResponse_OmitsUserInternalFields pins the plan's explicit
// instruction: never serialise auth.UserResult directly. Active and
// TokenVersion are auth internals -- TokenVersion in particular leaks
// revocation state -- and must never reach an auth response.
func TestToTokenResponse_OmitsUserInternalFields(t *testing.T) {
	userID := uuid.New()
	tp := &auth.TokenPair{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresIn:    900,
		User: auth.UserResult{
			ID:           userID,
			Email:        "user@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       false,  // internal -- must not reach the wire
			TokenVersion: 424242, // internal -- must not reach the wire
		},
	}

	got := toTokenResponse(tp)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"access_token", "refresh_token", "expires_in", "user"}, keysOf(fields),
		"the token response must expose exactly these fields")

	var userFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["user"], &userFields))
	assert.ElementsMatch(t, []string{"id", "email", "first_name", "last_name", "role"}, keysOf(userFields),
		"the embedded user must expose exactly these fields")

	assert.NotContains(t, string(raw), "424242",
		"token_version is auth-internal revocation state and must not be serialised")
	assert.NotContains(t, string(raw), `"active"`,
		"active is auth-internal and must not be serialised")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
