package paging_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestParseOffsetPage(t *testing.T) {
	t.Run("defaults when no params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		page := paging.ParseOffsetPage(req)

		assert.Equal(t, paging.OffsetPage{Page: 1, PageSize: 20}, page)
	})

	t.Run("parses explicit page and page_size", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=3&page_size=50", nil)
		page := paging.ParseOffsetPage(req)

		assert.Equal(t, paging.OffsetPage{Page: 3, PageSize: 50}, page)
	})

	t.Run("defaults on invalid values", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=abc&page_size=xyz", nil)
		page := paging.ParseOffsetPage(req)

		assert.Equal(t, paging.OffsetPage{Page: 1, PageSize: 20}, page)
	})

	t.Run("clamps page_size above max to default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page_size=200", nil)
		page := paging.ParseOffsetPage(req)

		assert.Equal(t, 20, page.PageSize)
	})

	t.Run("clamps page_size zero to default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page_size=0", nil)
		page := paging.ParseOffsetPage(req)

		assert.Equal(t, 20, page.PageSize)
	})
}

func TestOffsetPage_Offset(t *testing.T) {
	t.Run("first page", func(t *testing.T) {
		page := paging.OffsetPage{Page: 1, PageSize: 20}
		assert.Equal(t, 0, page.Offset())
	})

	t.Run("second page", func(t *testing.T) {
		page := paging.OffsetPage{Page: 2, PageSize: 20}
		assert.Equal(t, 20, page.Offset())
	})

	t.Run("third page size 10", func(t *testing.T) {
		page := paging.OffsetPage{Page: 3, PageSize: 10}
		assert.Equal(t, 20, page.Offset())
	})
}

func TestOffsetPage_Limit(t *testing.T) {
	page := paging.OffsetPage{Page: 1, PageSize: 25}
	assert.Equal(t, 25, page.Limit())
}

func TestNewOffsetPageResult(t *testing.T) {
	t.Run("middle page", func(t *testing.T) {
		items := []string{"a", "b", "c"}
		page := paging.OffsetPage{Page: 2, PageSize: 10}
		result := paging.NewOffsetPageResult(items, page, 25)

		expected := paging.OffsetPageResult[string]{
			Items: []string{"a", "b", "c"},
			Pagination: paging.OffsetPagination{
				CurrentPage: 2,
				PageSize:    10,
				TotalItems:  25,
				TotalPages:  3,
				HasPrevious: true,
				HasNext:     true,
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("first page", func(t *testing.T) {
		items := []string{"a"}
		page := paging.OffsetPage{Page: 1, PageSize: 10}
		result := paging.NewOffsetPageResult(items, page, 25)

		expected := paging.OffsetPageResult[string]{
			Items: []string{"a"},
			Pagination: paging.OffsetPagination{
				CurrentPage: 1,
				PageSize:    10,
				TotalItems:  25,
				TotalPages:  3,
				HasPrevious: false,
				HasNext:     true,
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("last page", func(t *testing.T) {
		items := []string{"a"}
		page := paging.OffsetPage{Page: 3, PageSize: 10}
		result := paging.NewOffsetPageResult(items, page, 25)

		expected := paging.OffsetPageResult[string]{
			Items: []string{"a"},
			Pagination: paging.OffsetPagination{
				CurrentPage: 3,
				PageSize:    10,
				TotalItems:  25,
				TotalPages:  3,
				HasPrevious: true,
				HasNext:     false,
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("nil items becomes empty slice", func(t *testing.T) {
		page := paging.OffsetPage{Page: 1, PageSize: 10}
		result := paging.NewOffsetPageResult[string](nil, page, 0)

		assert.NotNil(t, result.Items)
		assert.Empty(t, result.Items)
	})

	t.Run("exact division total pages", func(t *testing.T) {
		page := paging.OffsetPage{Page: 1, PageSize: 10}
		result := paging.NewOffsetPageResult([]string{"a"}, page, 30)

		assert.Equal(t, 3, result.Pagination.TotalPages)
	})
}
