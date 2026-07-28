package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/category"
)

// TestToCategoryResponse_ExposesExactFieldSet pins categoryResponse's wire
// shape. category.Category carries no fields that need hiding for security
// reasons (unlike user or payment), so this mirrors the domain model 1:1 --
// but the mapping is still explicit and still tested, so a field added to
// Category later does not silently ride onto the wire without this test
// being updated to acknowledge it.
func TestToCategoryResponse_ExposesExactFieldSet(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toCategoryResponse(&category.Category{
		ID:        uuid.New(),
		Name:      "Electronics",
		Slug:      "electronics",
		SortOrder: 3,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "name", "slug", "sort_order", "active", "created_at", "updated_at"},
		keysOf(fields),
		"description and parent_id are omitempty and absent when nil; every other Category field must be present")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
