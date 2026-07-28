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
//
// Description and ParentID are set too, and asserted *present*. They are the
// fields the public and admin mappers share, and both are `omitempty` -- so
// leaving them nil (as this fixture originally did) meant deleting
// `Description: c.Description` from either mapper broke nothing. Pinning them
// here makes the shared half of the duplicated mapping load-bearing.
func TestToCategoryResponse_OmitsModerationAndAuditFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "Phones, laptops and audio"
	parentID := uuid.New()
	got := toCategoryResponse(&category.Category{
		ID:          uuid.New(),
		Name:        "Electronics",
		Slug:        "electronics",
		Description: &description,
		ParentID:    &parentID,
		SortOrder:   3,
		Active:      true, // internal moderation state -- must not reach an anonymous caller
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "name", "slug", "description", "parent_id"}, keysOf(fields),
		"description and parent_id belong to the public shape and must be mapped; sort_order, active, and "+
			"the audit timestamps must never reach the public endpoint")
	assert.JSONEq(t, `"Phones, laptops and audio"`, string(fields["description"]),
		"description must carry the category's own value, not be dropped or defaulted")
}

// TestToAdminCategoryResponse_KeepsModerationAndAuditFields pins the
// complementary admin shape: an operator does need SortOrder, Active, and
// the audit timestamps to manage a category, so adminCategoryResponse (used
// by the admin Create/Update endpoints) must keep all of them.
func TestToAdminCategoryResponse_KeepsModerationAndAuditFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "Phones, laptops and audio"
	parentID := uuid.New()
	got := toAdminCategoryResponse(&category.Category{
		ID:          uuid.New(),
		Name:        "Electronics",
		Slug:        "electronics",
		Description: &description,
		ParentID:    &parentID,
		SortOrder:   3,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "name", "slug", "description", "parent_id",
			"sort_order", "active", "created_at", "updated_at",
		},
		keysOf(fields),
		"every Category field must be present for admin tooling")
	assert.JSONEq(t, `"Phones, laptops and audio"`, string(fields["description"]),
		"description must carry the category's own value -- this is one of the five field mappings "+
			"duplicated between toCategoryResponse and toAdminCategoryResponse, so it needs pinning on both")
	assert.JSONEq(t, `"`+parentID.String()+`"`, string(fields["parent_id"]),
		"parent_id must carry the category's own value")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
