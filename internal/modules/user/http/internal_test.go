package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/user"
)

// TestToUserResponse_OmitsCredentialAndAuthInternalFields pins the plan's
// highest-value mapper: PasswordHash and TokenVersion are credential
// material and revocation state, and Role/Active/the timestamps are
// deliberately reserved for the admin-only adminUserResponse. A typo that
// re-adds any of these to userResponse must fail this test.
func TestToUserResponse_OmitsCredentialAndAuthInternalFields(t *testing.T) {
	userID := uuid.New()
	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toUserResponse(&user.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: "$2a$10$distinguishablebcryptvalue", // credential material -- must not reach the wire
		FirstName:    "John",
		LastName:     "Doe",
		Phone:        "+15551234567",
		Role:         "admin", // reserved for adminUserResponse
		Active:       false,   // reserved for adminUserResponse
		TokenVersion: 424242,  // internal -- must not reach the wire
		CreatedAt:    deletedAt,
		UpdatedAt:    deletedAt,
		DeletedAt:    &deletedAt,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "email", "first_name", "last_name", "phone"}, keysOf(fields),
		"the public profile must expose exactly these fields")

	assert.NotContains(t, string(raw), "distinguishablebcryptvalue",
		"PasswordHash is credential material and must never be serialised")
	assert.NotContains(t, string(raw), "424242",
		"TokenVersion is auth-internal revocation state and must never be serialised")
	assert.NotContains(t, string(raw), `"role"`,
		"role is reserved for the admin response")
	assert.NotContains(t, string(raw), `"active"`,
		"active is reserved for the admin response")
	assert.NotContains(t, string(raw), "2026-01-01",
		"timestamps are reserved for the admin response")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestToAdminUserResponse_ExposesOperatorFieldsButNotCredentials pins the
// other half of the userResponse/adminUserResponse split: the admin shape
// legitimately carries role, active, and timestamps, but PasswordHash,
// TokenVersion, and DeletedAt remain off-limits everywhere.
func TestToAdminUserResponse_ExposesOperatorFieldsButNotCredentials(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deletedAt := now.Add(time.Hour)

	got := toAdminUserResponse(&user.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: "$2a$10$distinguishablebcryptvalue", // must not reach the wire
		FirstName:    "John",
		LastName:     "Doe",
		Phone:        "+15551234567",
		Role:         "admin",
		Active:       true,
		TokenVersion: 424242, // must not reach the wire
		CreatedAt:    now,
		UpdatedAt:    now,
		DeletedAt:    &deletedAt,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "email", "first_name", "last_name", "phone", "role", "active", "created_at", "updated_at"},
		keysOf(fields),
		"the admin response must expose exactly these fields")

	assert.NotContains(t, string(raw), "distinguishablebcryptvalue",
		"PasswordHash is credential material and must never be serialised, even to an admin")
	assert.NotContains(t, string(raw), "424242",
		"TokenVersion is auth-internal revocation state and must never be serialised")
}
