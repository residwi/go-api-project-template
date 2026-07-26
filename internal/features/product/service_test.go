package product_test

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
	"github.com/residwi/go-api-project-template/internal/features/product"
	mocks "github.com/residwi/go-api-project-template/mocks/product"
)

func TestService_Create(t *testing.T) {
	t.Run("success sets slug default currency and status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*product.Product")).
			Run(func(_ context.Context, p *product.Product) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)
		reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := svc.Create(context.Background(), product.CreateProductRequest{
			Name:  "Cool Widget",
			Price: 1999,
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &product.Product{
			Name:     "Cool Widget",
			Slug:     "cool-widget",
			Price:    1999,
			Currency: "USD",
			Status:   product.StatusDraft,
		}, result)
	})

	t.Run("sets currency and status from request", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p *product.Product) bool {
			return p.Currency == "EUR" && p.Status == product.StatusPublished
		})).Run(func(_ context.Context, p *product.Product) {
			p.ID = uuid.New()
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
		}).Return(nil)
		reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := svc.Create(context.Background(), product.CreateProductRequest{
			Name:     "Widget",
			Price:    1000,
			Currency: "EUR",
			Status:   product.StatusPublished,
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &product.Product{
			Name:     "Widget",
			Slug:     "widget",
			Price:    1000,
			Currency: "EUR",
			Status:   product.StatusPublished,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		p, err := svc.Create(context.Background(), product.CreateProductRequest{
			Name:  "Widget",
			Price: 1000,
		})
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_GetBySlug(t *testing.T) {
	t.Run("success only published", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetBySlug(mock.Anything, "cool-widget").
			Return(&product.Product{
				ID:     id,
				Name:   "Cool Widget",
				Slug:   "cool-widget",
				Status: product.StatusPublished,
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return([]product.Image{
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/1.jpg"},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]product.Availability{}, nil)

		p, err := svc.GetBySlug(context.Background(), "cool-widget")
		require.NoError(t, err)
		assert.Equal(t, "cool-widget", p.Slug)
		assert.Len(t, p.Images, 1)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").
			Return(nil, apperror.ErrNotFound)

		_, err := svc.GetBySlug(context.Background(), "nonexistent")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("draft product returns ErrNotFound", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetBySlug(mock.Anything, "draft-item").
			Return(&product.Product{
				ID:     uuid.New(),
				Slug:   "draft-item",
				Status: product.StatusDraft,
			}, nil)

		_, err := svc.GetBySlug(context.Background(), "draft-item")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetBySlug(mock.Anything, "widget").
			Return(&product.Product{
				ID:     id,
				Slug:   "widget",
				Status: product.StatusPublished,
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := svc.GetBySlug(context.Background(), "widget")
		require.Error(t, err)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Run("success loads images", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&product.Product{
				ID:   id,
				Name: "Widget",
				Slug: "widget",
			}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return([]product.Image{
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/a.jpg"},
				{ID: uuid.New(), ProductID: id, URL: "https://img.example.com/b.jpg"},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]product.Availability{}, nil)

		p, err := svc.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "Widget", p.Name)
		assert.Len(t, p.Images, 2)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		p, err := svc.GetByID(context.Background(), id)
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("images fetch error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&product.Product{ID: id, Name: "Widget"}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, id).
			Return(nil, errors.New("db error"))

		_, err := svc.GetByID(context.Background(), id)
		require.Error(t, err)
	})
}

func TestService_ListPublished(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		params := product.PublishedListParams{Limit: 10}
		products := []product.Product{
			{ID: id, Name: "A", Status: product.StatusPublished},
		}
		repo.EXPECT().ListPublished(mock.Anything, params).
			Return(products, "next-cursor", true, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]product.Availability{}, nil)

		result, cursor, hasMore, err := svc.ListPublished(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "next-cursor", cursor)
		assert.True(t, hasMore)
	})
}

func TestService_ListAdmin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		idA, idB := uuid.New(), uuid.New()
		params := product.AdminListParams{Page: 1, PageSize: 20}
		products := []product.Product{
			{ID: idA, Name: "A"},
			{ID: idB, Name: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).
			Return(products, 2, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{idA, idB}).
			Return(map[uuid.UUID]product.Availability{}, nil)

		result, total, err := svc.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}

func TestService_Update(t *testing.T) {
	t.Run("success partial update", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&product.Product{
				ID:       id,
				Name:     "Old Name",
				Slug:     "old-name",
				Price:    1000,
				Currency: "USD",
				Status:   product.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*product.Product")).Return(nil)

		newName := "New Name"
		p, err := svc.Update(context.Background(), id, product.UpdateProductRequest{
			Name: &newName,
		})
		require.NoError(t, err)
		assert.Equal(t, &product.Product{
			ID:       id,
			Name:     "New Name",
			Slug:     "new-name",
			Price:    1000,
			Currency: "USD",
			Status:   product.StatusDraft,
		}, p)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		catID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&product.Product{
				ID:       id,
				Name:     "Old",
				Slug:     "old",
				Price:    1000,
				Currency: "USD",
				Status:   product.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "New"
		newDesc := "A description"
		newPrice := int64(2000)
		newCompare := int64(2500)
		newCurrency := "EUR"
		newSKU := "SKU-001"
		newStatus := product.StatusPublished
		result, err := svc.Update(context.Background(), id, product.UpdateProductRequest{
			CategoryID:     &catID,
			Name:           &newName,
			Description:    &newDesc,
			Price:          &newPrice,
			CompareAtPrice: &newCompare,
			Currency:       &newCurrency,
			SKU:            &newSKU,
			Status:         &newStatus,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &product.Product{
			CategoryID:     &catID,
			Name:           "New",
			Slug:           "new",
			Description:    &newDesc,
			Price:          2000,
			CompareAtPrice: &newCompare,
			Currency:       "EUR",
			SKU:            &newSKU,
			Status:         product.StatusPublished,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.Update(context.Background(), uuid.New(), product.UpdateProductRequest{})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&product.Product{
				ID:       id,
				Name:     "Old",
				Slug:     "old",
				Price:    1000,
				Currency: "USD",
				Status:   product.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		newName := "New"
		p, err := svc.Update(context.Background(), id, product.UpdateProductRequest{
			Name: &newName,
		})
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().Delete(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(apperror.ErrNotFound)

		err := svc.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_AddImage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		productID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&product.Product{ID: productID}, nil)
		repo.EXPECT().AddImage(mock.Anything, mock.AnythingOfType("*product.Image")).
			Run(func(_ context.Context, img *product.Image) {
				img.ID = uuid.New()
				img.CreatedAt = time.Now()
			}).
			Return(nil)

		altText := "front view"
		sortOrder := 1
		img, err := svc.AddImage(context.Background(), productID, product.AddImageRequest{
			URL:       "https://img.example.com/front.jpg",
			AltText:   &altText,
			SortOrder: &sortOrder,
		})
		require.NoError(t, err)
		assert.Equal(t, productID, img.ProductID)
		assert.Equal(t, "https://img.example.com/front.jpg", img.URL)
		assert.Equal(t, 1, img.SortOrder)
	})

	t.Run("product not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.AddImage(context.Background(), uuid.New(), product.AddImageRequest{
			URL: "https://img.example.com/x.jpg",
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("add image repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		productID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&product.Product{ID: productID}, nil)
		repo.EXPECT().AddImage(mock.Anything, mock.Anything).Return(errors.New("db error"))

		_, err := svc.AddImage(context.Background(), productID, product.AddImageRequest{
			URL: "https://img.example.com/x.jpg",
		})
		require.Error(t, err)
	})
}

func TestService_DeleteImage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		productID := uuid.New()
		imageID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&product.Product{ID: productID}, nil)
		repo.EXPECT().DeleteImage(mock.Anything, imageID).Return(nil)

		err := svc.DeleteImage(context.Background(), productID, imageID)
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		err := svc.DeleteImage(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_ListPublished_EnrichesWithAvailability(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	inv := mocks.NewMockInventoryReader(t)
	reg := mocks.NewMockInventoryRegistrar(t)
	svc := product.NewService(repo, inv, reg)

	id1, id2 := uuid.New(), uuid.New()
	repo.EXPECT().ListPublished(mock.Anything, mock.Anything).
		Return([]product.Product{{ID: id1}, {ID: id2}}, "", false, nil)

	// One batch call for the whole page -- not one call per product.
	inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id1, id2}).
		Return(map[uuid.UUID]product.Availability{
			id1: {OnHand: 10, Available: 7},
			id2: {OnHand: 0, Available: 0},
		}, nil)

	items, _, _, err := svc.ListPublished(context.Background(), product.PublishedListParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 7, items[0].Availability.Available)
	assert.Equal(t, 0, items[1].Availability.Available)
}

func TestService_GetByIDsIncludingDeleted(t *testing.T) {
	t.Run("enriches through inventory in one batch call", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]product.Product{
				{ID: liveID, Name: "Live", Status: product.StatusPublished},
				{ID: archivedID, Name: "Gone", Status: product.StatusArchived},
			}, nil)
		// One batch call for the whole set -- not one call per id.
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{liveID, archivedID}).
			Return(map[uuid.UUID]product.Availability{
				liveID: {OnHand: 10, Available: 8},
			}, nil)

		got, err := svc.GetByIDsIncludingDeleted(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, 8, got[0].Availability.Available)
		assert.Equal(t, product.StatusArchived, got[1].Status)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		_, err := svc.GetByIDsIncludingDeleted(context.Background(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestService_AvailableQuantity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&product.Product{ID: id}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]product.Availability{
				id: {OnHand: 130, Available: 70},
			}, nil)

		avail, err := svc.AvailableQuantity(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, 70, avail)
	})

	t.Run("negative available returns ErrInsufficientStock", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&product.Product{ID: id}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, []uuid.UUID{id}).
			Return(map[uuid.UUID]product.Availability{
				id: {OnHand: 5, Available: -5},
			}, nil)

		_, err := svc.AvailableQuantity(context.Background(), id)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		inv := mocks.NewMockInventoryReader(t)
		reg := mocks.NewMockInventoryRegistrar(t)
		svc := product.NewService(repo, inv, reg)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		_, err := svc.AvailableQuantity(context.Background(), id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_Create_RegistersZeroInventoryLevel(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	inv := mocks.NewMockInventoryReader(t)
	reg := mocks.NewMockInventoryRegistrar(t)
	svc := product.NewService(repo, inv, reg)

	repo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, p *product.Product) error {
			p.ID = uuid.New()
			return nil
		})
	// A new product must be registered with inventory, or its first reservation
	// would fail on a missing level row.
	reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)
	// No GetAvailability expectation: Create must not read inventory back for a
	// value it already knows by construction. inv has no expectation set, so
	// mockery fails this test immediately if Create calls it anyway.

	p, err := svc.Create(context.Background(), product.CreateProductRequest{
		Name:  "Widget",
		Price: 1500,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
}
