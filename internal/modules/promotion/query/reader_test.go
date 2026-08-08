package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		params := Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		promos := []domain.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(promos, 2, nil)

		result, total, err := reader.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		params := Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, assert.AnError)

		_, _, err := reader.ListAdmin(context.Background(), params)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
