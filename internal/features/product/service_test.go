package product

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success sets slug default currency and status", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Product")).
			Run(func(_ context.Context, p *domain.Product) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)
		inv.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := s.Create(context.Background(), nil, "Cool Widget", nil, money.New(1999, ""), nil, nil, "")
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Product{
			Name:   "Cool Widget",
			Slug:   "cool-widget",
			Price:  money.New(1999, "USD"),
			Status: domain.StatusDraft,
		}, result)
	})

	t.Run("sets currency and status from request", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p *domain.Product) bool {
			return p.Price.Currency == "EUR" && p.Status == domain.StatusPublished
		})).Run(func(_ context.Context, p *domain.Product) {
			p.ID = uuid.New()
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
		}).Return(nil)
		inv.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := s.Create(
			context.Background(), nil, "Widget", nil, money.New(1000, "EUR"), nil, nil, domain.StatusPublished,
		)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Product{
			Name:   "Widget",
			Slug:   "widget",
			Price:  money.New(1000, "EUR"),
			Status: domain.StatusPublished,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errs.ErrConflict)

		p, err := s.Create(context.Background(), nil, "Widget", nil, money.New(1000, ""), nil, nil, "")
		assert.Nil(t, p)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_Create_RegistersZeroInventoryLevel(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	inv := NewMockInventory(t)
	s := New(repo, inv)

	repo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, p *domain.Product) error {
			p.ID = uuid.New()
			return nil
		})
	inv.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)
	// No GetAvailability expectation -- Create only calls EnsureLevel, so mockery
	// fails the test if Create reads inventory back for a value it already knows
	// by construction.

	p, err := s.Create(context.Background(), nil, "Widget", nil, money.New(1500, ""), nil, nil, "")
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestService_Update(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{
				ID:     id,
				Name:   "Old Name",
				Slug:   "old-name",
				Price:  money.New(1000, "USD"),
				Status: domain.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.Product")).Return(nil)

		newName := "New Name"
		p, err := s.Update(context.Background(), id, nil, &newName, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, &domain.Product{
			ID:     id,
			Name:   "New Name",
			Slug:   "new-name",
			Price:  money.New(1000, "USD"),
			Status: domain.StatusDraft,
		}, p)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		id := uuid.New()
		catID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{
				ID:     id,
				Name:   "Old",
				Slug:   "old",
				Price:  money.New(1000, "USD"),
				Status: domain.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "New"
		newDesc := "A description"
		newPrice := money.New(2000, "EUR")
		// Deliberately the old currency: products stores one currency column for both
		// amounts, so Update must restate the compare-at price in the new one.
		newCompare := money.New(2500, "USD")
		newSKU := "SKU-001"
		newStatus := domain.StatusPublished
		result, err := s.Update(
			context.Background(), id, &catID, &newName, &newDesc, &newPrice, &newCompare, &newSKU, &newStatus,
		)

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		expectedCompare := money.New(2500, "EUR")
		assert.Equal(t, &domain.Product{
			CategoryID:     &catID,
			Name:           "New",
			Slug:           "new",
			Description:    &newDesc,
			Price:          money.New(2000, "EUR"),
			CompareAtPrice: &expectedCompare,
			SKU:            &newSKU,
			Status:         domain.StatusPublished,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.Update(context.Background(), uuid.New(), nil, nil, nil, nil, nil, nil, nil)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{
				ID:     id,
				Name:   "Old",
				Slug:   "old",
				Price:  money.New(1000, "USD"),
				Status: domain.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(errs.ErrConflict)

		newName := "New"
		p, err := s.Update(context.Background(), id, nil, &newName, nil, nil, nil, nil, nil)
		assert.Nil(t, p)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_Update_RepricesStoredCompareAtPriceIntoTheNewCurrency(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	var inv Inventory
	s := New(repo, inv)

	id := uuid.New()
	storedCompare := money.New(1500, "USD")
	repo.EXPECT().GetByID(mock.Anything, id).
		Return(&domain.Product{
			ID:             id,
			Name:           "Widget",
			Slug:           "widget",
			Price:          money.New(1000, "USD"),
			CompareAtPrice: &storedCompare,
			Status:         domain.StatusPublished,
		}, nil)

	var persisted *domain.Product
	repo.EXPECT().Update(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *domain.Product) { persisted = p }).
		Return(nil)

	newPrice := money.New(900, "EUR")
	result, err := s.Update(context.Background(), id, nil, nil, nil, &newPrice, nil, nil, nil)
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.CompareAtPrice)
	assert.Equal(t, money.New(1500, "EUR"), *persisted.CompareAtPrice,
		"the stored compare-at price keeps its amount but must be restated in the new currency")
	assert.Equal(t, money.New(900, "EUR"), persisted.Price)

	require.NotNil(t, result.CompareAtPrice)
	assert.Equal(t, result.Price.Currency, result.CompareAtPrice.Currency,
		"both amounts must share a currency -- the row does, so the returned struct must too")
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := s.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		repo.EXPECT().Delete(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(errs.ErrNotFound)

		err := s.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_AddImage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		productID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&domain.Product{ID: productID}, nil)
		repo.EXPECT().AddImage(mock.Anything, mock.AnythingOfType("*domain.Image")).
			Run(func(_ context.Context, img *domain.Image) {
				img.ID = uuid.New()
				img.CreatedAt = time.Now()
			}).
			Return(nil)

		altText := "front view"
		sortOrder := 1
		img, err := s.AddImage(
			context.Background(), productID, "https://img.example.com/front.jpg", &altText, &sortOrder,
		)
		require.NoError(t, err)
		assert.Equal(t, productID, img.ProductID)
		assert.Equal(t, "https://img.example.com/front.jpg", img.URL)
		assert.Equal(t, 1, img.SortOrder)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.AddImage(context.Background(), uuid.New(), "https://img.example.com/x.jpg", nil, nil)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("add image repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		productID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&domain.Product{ID: productID}, nil)
		repo.EXPECT().AddImage(mock.Anything, mock.Anything).Return(errors.New("db error"))

		_, err := s.AddImage(context.Background(), productID, "https://img.example.com/x.jpg", nil, nil)
		require.Error(t, err)
	})
}

func TestService_DeleteImage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		productID := uuid.New()
		imageID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&domain.Product{ID: productID}, nil)
		repo.EXPECT().DeleteImage(mock.Anything, imageID).Return(nil)

		err := s.DeleteImage(context.Background(), productID, imageID)
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		err := s.DeleteImage(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success only published", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

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
			Return(map[uuid.UUID]inventory.Availability{}, nil)

		p, err := s.GetBySlug(context.Background(), "cool-widget")
		require.NoError(t, err)
		assert.Equal(t, "cool-widget", p.Slug)
		assert.Len(t, p.Images, 1)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").
			Return(nil, errs.ErrNotFound)

		_, err := s.GetBySlug(context.Background(), "nonexistent")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("draft product returns ErrNotFound", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().GetBySlug(mock.Anything, "draft-item").
			Return(&domain.Product{
				ID:     uuid.New(),
				Slug:   "draft-item",
				Status: domain.StatusDraft,
			}, nil)

		_, err := s.GetBySlug(context.Background(), "draft-item")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetBySlug(mock.Anything, "widget").
			Return(&domain.Product{
				ID:     id,
				Slug:   "widget",
				Status: domain.StatusPublished,
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := s.GetBySlug(context.Background(), "widget")
		require.Error(t, err)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success loads images", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

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
			Return(map[uuid.UUID]inventory.Availability{}, nil)

		p, err := s.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "Widget", p.Name)
		assert.Len(t, p.Images, 2)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, errs.ErrNotFound)

		p, err := s.GetByID(context.Background(), id)
		assert.Nil(t, p)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{ID: id, Name: "Widget"}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := s.GetByID(context.Background(), id)
		require.Error(t, err)
	})
}

func TestService_ListPublished(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		id := uuid.New()
		params := PublishedListParams{Limit: 10}
		products := []domain.Product{
			{ID: id, Name: "A", Status: domain.StatusPublished},
		}
		repo.EXPECT().ListPublished(mock.Anything, params).
			Return(products, "next-cursor", true, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]inventory.Availability{}, nil)

		result, cursor, hasMore, err := s.ListPublished(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "next-cursor", cursor)
		assert.True(t, hasMore)
	})
}

func TestService_ListPublished_EnrichesWithAvailability(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	inv := NewMockInventory(t)
	s := New(repo, inv)

	id1, id2 := uuid.New(), uuid.New()
	repo.EXPECT().ListPublished(mock.Anything, mock.Anything).
		Return([]domain.Product{{ID: id1}, {ID: id2}}, "", false, nil)

	// One batch call for the whole page -- not one call per product.
	inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id1, id2}).
		Return(map[uuid.UUID]inventory.Availability{
			id1: {OnHand: 10, Available: 7},
			id2: {OnHand: 0, Available: 0},
		}, nil)

	items, _, _, err := s.ListPublished(context.Background(), PublishedListParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 7, items[0].Availability.Available)
	assert.Equal(t, 0, items[1].Availability.Available)
}

func TestService_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		idA, idB := uuid.New(), uuid.New()
		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}
		products := []domain.Product{
			{ID: idA, Name: "A"},
			{ID: idB, Name: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).
			Return(products, 2, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{idA, idB}).
			Return(map[uuid.UUID]inventory.Availability{}, nil)

		result, total, err := s.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}

func TestService_getByIDsIncludingDeleted(t *testing.T) {
	t.Parallel()

	t.Run("enriches through inventory in one batch call", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]domain.Product{
				{ID: liveID, Name: "Live", Status: domain.StatusPublished},
				{ID: archivedID, Name: "Gone", Status: domain.StatusArchived},
			}, nil)
		// One batch call for the whole set -- not one call per id.
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{liveID, archivedID}).
			Return(map[uuid.UUID]inventory.Availability{
				liveID: {OnHand: 10, Available: 8},
			}, nil)

		got, err := s.getByIDsIncludingDeleted(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, 8, got[0].Availability.Available)
		assert.Equal(t, domain.StatusArchived, got[1].Status)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		_, err := s.getByIDsIncludingDeleted(context.Background(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestService_CountPublished(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		categoryID := uuid.New()
		repo.EXPECT().CountPublishedByCategory(mock.Anything, categoryID).Return(3, nil)

		count, err := s.CountPublished(context.Background(), categoryID)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var inv Inventory
		s := New(repo, inv)

		categoryID := uuid.New()
		repo.EXPECT().CountPublishedByCategory(mock.Anything, categoryID).Return(0, errors.New("db error"))

		_, err := s.CountPublished(context.Background(), categoryID)
		assert.Error(t, err)
	})
}

func TestService_GetInfoByIDs(t *testing.T) {
	t.Parallel()

	t.Run("maps a batch in one call and carries an unaffected status through", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]domain.Product{
				{ID: liveID, Name: "Widget", Price: money.New(1500, "USD"), Status: domain.StatusPublished},
				{ID: archivedID, Name: "Gone", Price: money.New(900, "USD"), Status: domain.StatusArchived},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, ids).
			Return(map[uuid.UUID]inventory.Availability{
				liveID: {OnHand: 10, Available: 7},
			}, nil)

		got, err := s.GetInfoByIDs(context.Background(), ids)
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

		inv := NewMockInventory(t)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]inventory.Availability{productID: {OnHand: 5, Available: 5}}, nil)

		s := New(repo, inv)

		got, err := s.GetInfoByIDs(context.Background(), []uuid.UUID{productID})

		require.NoError(t, err)
		assert.Equal(t, map[uuid.UUID]Info{
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

		inv := NewMockInventory(t)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]inventory.Availability{productID: {OnHand: 2, Available: 2}}, nil)

		s := New(repo, inv)

		got, err := s.GetInfoByIDs(context.Background(), []uuid.UUID{productID})

		require.NoError(t, err)
		assert.Equal(t, domain.StatusPublished, got[productID].Status)
	})

	t.Run("propagates a repository error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		inv := NewMockInventory(t)
		s := New(repo, inv)

		ids := []uuid.UUID{uuid.New()}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).Return(nil, errors.New("db error"))

		got, err := s.GetInfoByIDs(context.Background(), ids)
		assert.Nil(t, got)
		assert.Error(t, err)
	})
}
