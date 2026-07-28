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

// TestToCategoryResponse_OmitsModerationAndAuditFields pins the public
// categoryResponse's wire shape. An anonymous caller has no legitimate
// reason to see whether a category is still staged (Active) -- GET
// /categories and GET /categories/{slug} are unauthenticated and the
// repository's List has no WHERE active filter, so this field is the only
// thing standing between an anonymous caller and enumerating unpublished
// categories. SortOrder and the audit timestamps are dropped alongside it as
// merchandising/admin details a shopper has no use for. The fixture sets
// every one of them to a non-zero value, so this fails if any is re-added.
func TestToCategoryResponse_OmitsModerationAndAuditFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toCategoryResponse(&category.Category{
		ID:        uuid.New(),
		Name:      "Electronics",
		Slug:      "electronics",
		SortOrder: 3,
		Active:    true, // internal moderation state -- must not reach an anonymous caller
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "name", "slug"}, keysOf(fields),
		"description and parent_id are omitempty and absent when nil; sort_order, active, and the audit "+
			"timestamps must never reach the public endpoint")
}

// TestToAdminCategoryResponse_KeepsModerationAndAuditFields pins the
// complementary admin shape: an operator does need SortOrder, Active, and
// the audit timestamps to manage a category, so adminCategoryResponse (used
// by the admin Create/Update endpoints) must keep all of them.
func TestToAdminCategoryResponse_KeepsModerationAndAuditFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toAdminCategoryResponse(&category.Category{
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
		"description and parent_id are omitempty and absent when nil; every other Category field must be "+
			"present for admin tooling")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
