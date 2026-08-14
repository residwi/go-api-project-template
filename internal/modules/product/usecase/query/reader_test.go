package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success only published", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetBySlug(mock.Anything, "cool-widget").
			Return(&domain.Product{
				ID:     id,
				Name:   "Cool Widget",
				Slug:   "cool-widget",
				Status: domain.StatusPublished,
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return([]domain.Image{
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/1.jpg"},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]inventorycontract.Availability{}, nil)

		p, err := r.GetBySlug(context.Background(), "cool-widget")
		require.NoError(t, err)
		assert.Equal(t, "cool-widget", p.Slug)
		assert.Len(t, p.Images, 1)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").
			Return(nil, apperror.ErrNotFound)

		_, err := r.GetBySlug(context.Background(), "nonexistent")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("draft product returns ErrNotFound", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		repo.EXPECT().GetBySlug(mock.Anything, "draft-item").
			Return(&domain.Product{
				ID:     uuid.New(),
				Slug:   "draft-item",
				Status: domain.StatusDraft,
			}, nil)

		_, err := r.GetBySlug(context.Background(), "draft-item")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetBySlug(mock.Anything, "widget").
			Return(&domain.Product{
				ID:     id,
				Slug:   "widget",
				Status: domain.StatusPublished,
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := r.GetBySlug(context.Background(), "widget")
		require.Error(t, err)
	})
}

func TestReader_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success loads images", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{
				ID:   id,
				Name: "Widget",
				Slug: "widget",
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return([]domain.Image{
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/a.jpg"},
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/b.jpg"},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]inventorycontract.Availability{}, nil)

		p, err := r.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "Widget", p.Name)
		assert.Len(t, p.Images, 2)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		p, err := r.GetByID(context.Background(), id)
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{ID: id, Name: "Widget"}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := r.GetByID(context.Background(), id)
		require.Error(t, err)
	})
}

func TestReader_ListPublished(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		id := uuid.New()
		params := PublishedListParams{Limit: 10}
		products := []domain.Product{
			{ID: id, Name: "A", Status: domain.StatusPublished},
		}
		repo.EXPECT().ListPublished(mock.Anything, params).
			Return(products, "next-cursor", true, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]inventorycontract.Availability{}, nil)

		result, cursor, hasMore, err := r.ListPublished(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "next-cursor", cursor)
		assert.True(t, hasMore)
	})
}

func TestReader_ListPublished_EnrichesWithAvailability(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	inv := NewMockInventoryReader(t)
	r := New(repo, inv)

	id1, id2 := uuid.New(), uuid.New()
	repo.EXPECT().ListPublished(mock.Anything, mock.Anything).
		Return([]domain.Product{{ID: id1}, {ID: id2}}, "", false, nil)

	// One batch call for the whole page -- not one call per product.
	inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id1, id2}).
		Return(map[uuid.UUID]inventorycontract.Availability{
			id1: {OnHand: 10, Available: 7},
			id2: {OnHand: 0, Available: 0},
		}, nil)

	items, _, _, err := r.ListPublished(context.Background(), PublishedListParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 7, items[0].Availability.Available)
	assert.Equal(t, 0, items[1].Availability.Available)
}

func TestReader_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		idA, idB := uuid.New(), uuid.New()
		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}
		products := []domain.Product{
			{ID: idA, Name: "A"},
			{ID: idB, Name: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).
			Return(products, 2, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{idA, idB}).
			Return(map[uuid.UUID]inventorycontract.Availability{}, nil)

		result, total, err := r.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}

func TestReader_GetByIDsIncludingDeleted(t *testing.T) {
	t.Parallel()

	t.Run("enriches through inventory in one batch call", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]domain.Product{
				{ID: liveID, Name: "Live", Status: domain.StatusPublished},
				{ID: archivedID, Name: "Gone", Status: domain.StatusArchived},
			}, nil)
		// One batch call for the whole set -- not one call per id.
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{liveID, archivedID}).
			Return(map[uuid.UUID]inventorycontract.Availability{
				liveID: {OnHand: 10, Available: 8},
			}, nil)

		got, err := r.GetByIDsIncludingDeleted(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, 8, got[0].Availability.Available)
		assert.Equal(t, domain.StatusArchived, got[1].Status)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		_, err := r.GetByIDsIncludingDeleted(context.Background(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestReader_CountPublished(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		categoryID := uuid.New()
		repo.EXPECT().CountPublishedByCategory(mock.Anything, categoryID).Return(3, nil)

		count, err := r.CountPublished(context.Background(), categoryID)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		categoryID := uuid.New()
		repo.EXPECT().CountPublishedByCategory(mock.Anything, categoryID).Return(0, errors.New("db error"))

		_, err := r.CountPublished(context.Background(), categoryID)
		assert.Error(t, err)
	})
}

func TestReaderGetInfoByIDs(t *testing.T) {
	t.Parallel()

	t.Run("maps a batch in one call and carries an unaffected status through", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]domain.Product{
				{ID: liveID, Name: "Widget", Price: money.New(1500, "USD"), Status: domain.StatusPublished},
				{ID: archivedID, Name: "Gone", Price: money.New(900, "USD"), Status: domain.StatusArchived},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, ids).
			Return(map[uuid.UUID]inventorycontract.Availability{
				liveID: {OnHand: 10, Available: 7},
			}, nil)

		got, err := r.GetInfoByIDs(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, 7, got[liveID].Available, "Available must come from Availability.Available")
		assert.Equal(t, domain.StatusArchived, got[archivedID].Status,
			"archived products must still come back, carrying Status")
	})

	t.Run("reports a withdrawn product as unavailable even though its status is still published", func(t *testing.T) {
		t.Parallel()

		productID := uuid.New()
		deletedAt := time.Now()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, []uuid.UUID{productID}).
			Return([]domain.Product{{
				ID:        productID,
				Name:      "Withdrawn Widget",
				Price:     money.New(1000, "IDR"),
				Status:    domain.StatusPublished,
				DeletedAt: &deletedAt,
			}}, nil)

		inv := NewMockInventoryReader(t)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]inventorycontract.Availability{productID: {OnHand: 5, Available: 5}}, nil)

		r := New(repo, inv)

		got, err := r.GetInfoByIDs(context.Background(), []uuid.UUID{productID})

		require.NoError(t, err)
		assert.Equal(t, map[uuid.UUID]contract.Product{
			productID: {
				ID:        productID,
				Name:      "Withdrawn Widget",
				Price:     money.New(1000, "IDR"),
				Status:    "unavailable",
				Available: 5,
			},
		}, got)
	})

	t.Run("passes a live product's status through unchanged", func(t *testing.T) {
		t.Parallel()

		productID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, []uuid.UUID{productID}).
			Return([]domain.Product{{
				ID:     productID,
				Name:   "Live Widget",
				Price:  money.New(2500, "IDR"),
				Status: domain.StatusPublished,
			}}, nil)

		inv := NewMockInventoryReader(t)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]inventorycontract.Availability{productID: {OnHand: 2, Available: 2}}, nil)

		r := New(repo, inv)

		got, err := r.GetInfoByIDs(context.Background(), []uuid.UUID{productID})

		require.NoError(t, err)
		assert.Equal(t, domain.StatusPublished, got[productID].Status)
	})

	t.Run("propagates a repository error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventoryReader(t)
		r := New(repo, inv)

		ids := []uuid.UUID{uuid.New()}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).Return(nil, errors.New("db error"))

		got, err := r.GetInfoByIDs(context.Background(), ids)
		assert.Nil(t, got)
		assert.Error(t, err)
	})
}
