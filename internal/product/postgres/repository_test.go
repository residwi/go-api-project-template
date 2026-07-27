package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/product/postgres"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_product")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedProduct(t *testing.T) *product.Product {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})

	repo := postgres.New(testPool)
	p, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return p
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates product", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		desc := "A description"
		p := &product.Product{
			Name:        "New Product",
			Slug:        "new-product-" + uuid.New().String(),
			Description: &desc,
			Price:       1000,
			Currency:    "USD",
			Status:      product.StatusPublished,
		}

		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.False(t, p.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, p.ID)
		})
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		setup(t)
		existing := seedProduct(t)
		repo := postgres.New(testPool)

		dup := &product.Product{
			Name:     "Duplicate",
			Slug:     existing.Slug,
			Price:    500,
			Currency: "USD",
			Status:   product.StatusDraft,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns product", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Name, got.Name)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetBySlug(t *testing.T) {
	t.Run("returns product by slug", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)

		got, err := repo.GetBySlug(context.Background(), p.Slug)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		_, err := repo.GetBySlug(context.Background(), "nonexistent-slug")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates product fields", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)

		p.Name = "Updated Product"
		p.Price = 2000
		p.Status = product.StatusArchived
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Product", got.Name)
		assert.Equal(t, int64(2000), got.Price)
		assert.Equal(t, product.StatusArchived, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		p := &product.Product{
			ID:       uuid.New(),
			Name:     "Ghost",
			Slug:     "ghost-" + uuid.New().String(),
			Price:    100,
			Currency: "USD",
			Status:   product.StatusDraft,
		}
		err := repo.Update(context.Background(), p)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		setup(t)
		p1 := seedProduct(t)
		p2 := seedProduct(t)
		repo := postgres.New(testPool)

		p2.Slug = p1.Slug
		err := repo.Update(context.Background(), p2)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes product", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)

		err := repo.Delete(context.Background(), p.ID)
		require.NoError(t, err)
	})

	t.Run("returns not found after delete", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, p.ID))

		_, err := repo.GetByID(ctx, p.ID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found for nonexistent product", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_ListPublished(t *testing.T) {
	t.Run("returns published products", func(t *testing.T) {
		setup(t)
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Published Product', $2, 'desc', 1000, 'USD', 'published')`,
			id, "pub-"+id.String(),
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
		})

		repo := postgres.New(testPool)
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Limit: 10,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products)
		for _, p := range products {
			assert.Equal(t, product.StatusPublished, p.Status)
		}
	})

	t.Run("filters by category ID", func(t *testing.T) {
		setup(t)
		catID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Filter Cat', $2, true)`,
			catID, "filter-cat-"+catID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, catID) })

		id := uuid.New()
		_, err = testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
			 VALUES ($1, 'Cat Product', $2, 'desc', 1000, 'USD', 'published', $3)`,
			id, "cat-prod-"+id.String()[:8], catID)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			CategoryID: &catID,
			Limit:      10,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products)
		for _, p := range products {
			assert.Equal(t, &catID, p.CategoryID)
		}
	})

	t.Run("filters by search", func(t *testing.T) {
		setup(t)
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'UniqueSearchable Widget', $2, 'desc', 1000, 'USD', 'published')`,
			id, "search-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Search: "UniqueSearchable",
			Limit:  10,
		})
		require.NoError(t, err)
		require.NotEmpty(t, products)
		assert.Equal(t, id, products[0].ID)
	})

	t.Run("filters by min and max price", func(t *testing.T) {
		setup(t)
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Price Filter Product', $2, 'desc', 5000, 'USD', 'published')`,
			id, "price-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		minPrice := int64(4000)
		maxPrice := int64(6000)
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			MinPrice: &minPrice,
			MaxPrice: &maxPrice,
			Limit:    10,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products)
		for _, p := range products {
			assert.GreaterOrEqual(t, p.Price, minPrice)
			assert.LessOrEqual(t, p.Price, maxPrice)
		}
	})

	t.Run("cursor pagination", func(t *testing.T) {
		setup(t)
		// Seed 3 published products
		for range 3 {
			id := uuid.New()
			_, err := testPool.Exec(context.Background(),
				`INSERT INTO products (id, name, slug, description, price, currency, status)
				 VALUES ($1, $2, $3, 'desc', 1000, 'USD', 'published')`,
				id, "cursor-prod-"+id.String()[:8], "cursor-"+id.String()[:8])
			require.NoError(t, err)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
		}

		repo := postgres.New(testPool)

		// First page
		products, nextCursor, hasMore, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Limit: 2,
		})
		require.NoError(t, err)
		assert.True(t, hasMore)
		assert.NotEmpty(t, nextCursor)
		assert.Len(t, products, 2)

		// Second page using cursor
		products2, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Cursor: nextCursor,
			Limit:  2,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products2)
		// Ensure no overlap
		for _, p2 := range products2 {
			for _, p1 := range products {
				assert.NotEqual(t, p1.ID, p2.ID)
			}
		}
	})
}

func TestPostgresRepository_ListPublished_InvalidCursor(t *testing.T) {
	t.Run("returns bad request error on invalid cursor", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		_, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Cursor: "invalid-cursor-value",
			Limit:  10,
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns all products with total", func(t *testing.T) {
		setup(t)
		seedProduct(t)
		seedProduct(t)
		repo := postgres.New(testPool)

		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		assert.NotEmpty(t, products)
	})

	t.Run("filters by status", func(t *testing.T) {
		setup(t)
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Draft Product', $2, 'desc', 1000, 'USD', 'draft')`,
			id, "draft-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			Page:     1,
			PageSize: 10,
			Status:   "draft",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range products {
			assert.Equal(t, product.StatusDraft, p.Status)
		}
	})

	t.Run("filters by category ID", func(t *testing.T) {
		setup(t)
		catID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Admin Cat', $2, true)`,
			catID, "admin-cat-"+catID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, catID) })

		id := uuid.New()
		_, err = testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
			 VALUES ($1, 'Admin Cat Product', $2, 'desc', 1000, 'USD', 'draft', $3)`,
			id, "admin-cat-prod-"+id.String()[:8], catID)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			Page:       1,
			PageSize:   10,
			CategoryID: &catID,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range products {
			assert.Equal(t, &catID, p.CategoryID)
		}
	})

	t.Run("filters by search on name, description, and sku", func(t *testing.T) {
		setup(t)
		sku := "UNIQSKU-" + uuid.New().String()[:8]
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, sku)
			 VALUES ($1, 'Admin Search Product', $2, 'desc', 1000, 'USD', 'draft', $3)`,
			id, "admin-search-"+id.String()[:8], sku)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := postgres.New(testPool)
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			Page:     1,
			PageSize: 10,
			Search:   sku,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, products)
		assert.Equal(t, id, products[0].ID)
	})
}

func TestPostgresRepository_GetByIDsIncludingDeleted(t *testing.T) {
	t.Run("returns soft-deleted and unpublished products alongside live ones", func(t *testing.T) {
		setup(t)
		live := seedProduct(t)

		archivedID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Archived', $2, 'desc', 1000, 'USD', 'archived')`,
			archivedID, "archived-"+archivedID.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, archivedID) })

		// status is left 'published': product.Delete only sets deleted_at, so this
		// mirrors a real withdrawn-but-still-published row.
		deletedID := uuid.New()
		_, err = testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, deleted_at)
			 VALUES ($1, 'Deleted', $2, 'desc', 1000, 'USD', 'published', NOW())`,
			deletedID, "deleted-"+deletedID.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, deletedID) })

		repo := postgres.New(testPool)
		got, err := repo.GetByIDsIncludingDeleted(context.Background(), []uuid.UUID{live.ID, archivedID, deletedID, uuid.New()})
		require.NoError(t, err)

		byID := make(map[uuid.UUID]product.Product, len(got))
		for _, p := range got {
			byID[p.ID] = p
		}
		require.Len(t, byID, 3, "the fourth id has no row at all and must simply be absent")
		assert.Equal(t, live.Status, byID[live.ID].Status)
		assert.Nil(t, byID[live.ID].DeletedAt)
		assert.Equal(t, "archived", byID[archivedID].Status)
		assert.Nil(t, byID[archivedID].DeletedAt)
		assert.Contains(t, byID, deletedID, "a soft-deleted product must still be returned")
		// The row is left with status='published' by product.Delete (only
		// deleted_at changes), so a caller must consult DeletedAt -- not Status --
		// to know this product is no longer sellable.
		assert.Equal(t, "published", byID[deletedID].Status)
		require.NotNil(t, byID[deletedID].DeletedAt, "a soft-deleted product must carry DeletedAt so a consumer can flag it unsellable")
	})

	t.Run("returns empty slice for empty ids", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		got, err := repo.GetByIDsIncludingDeleted(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPostgresRepository_CountPublishedByCategory(t *testing.T) {
	t.Run("returns zero when no products", func(t *testing.T) {
		setup(t)
		catID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Empty Cat', $2, true)`,
			catID, "empty-cat-"+catID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, catID) })

		repo := postgres.New(testPool)
		count, err := repo.CountPublishedByCategory(context.Background(), catID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns count of published products", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		catID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Count Cat', $2, true)`,
			catID, "count-cat-"+catID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

		productID := uuid.New()
		_, err = testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
			 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD', 'published', $3)`,
			productID, "slug-"+productID.String(), catID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, productID) })

		repo := postgres.New(testPool)
		count, err := repo.CountPublishedByCategory(ctx, catID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := postgres.New(testPool)

	t.Run("Create", func(t *testing.T) {
		setup(t)
		p := &product.Product{Name: "X", Slug: "x-" + uuid.New().String(), Price: 100, Currency: "USD", Status: product.StatusDraft}
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetBySlug", func(t *testing.T) {
		setup(t)
		_, err := repo.GetBySlug(cancelledCtx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		setup(t)
		p := &product.Product{ID: uuid.New(), Name: "X", Slug: "x-" + uuid.New().String(), Price: 100, Currency: "USD", Status: product.StatusDraft}
		err := repo.Update(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		setup(t)
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListPublished", func(t *testing.T) {
		setup(t)
		_, _, _, err := repo.ListPublished(cancelledCtx, product.PublishedListParams{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		setup(t)
		_, _, err := repo.ListAdmin(cancelledCtx, product.AdminListParams{Page: 1, PageSize: 10})
		assert.Error(t, err)
	})

	t.Run("GetByIDsIncludingDeleted", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByIDsIncludingDeleted(cancelledCtx, []uuid.UUID{uuid.New()})
		assert.Error(t, err)
	})

	t.Run("AddImage", func(t *testing.T) {
		setup(t)
		img := &product.Image{ProductID: uuid.New(), URL: "https://example.com/img.jpg", SortOrder: 0}
		err := repo.AddImage(cancelledCtx, img)
		assert.Error(t, err)
	})

	t.Run("DeleteImage", func(t *testing.T) {
		setup(t)
		err := repo.DeleteImage(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetImagesByProductID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetImagesByProductID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("CountPublishedByCategory", func(t *testing.T) {
		setup(t)
		_, err := repo.CountPublishedByCategory(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestSearchString(t *testing.T) {
	t.Run("returns false when substring not found", func(t *testing.T) {
		setup(t)
		existing := seedProduct(t)
		repo := postgres.New(testPool)

		// Update without changing slug — no unique violation, exercises isUniqueViolation returning false
		existing.Name = "Updated Name"
		err := repo.Update(context.Background(), existing)
		require.NoError(t, err)
	})
}

func TestPostgresRepository_Images(t *testing.T) {
	t.Run("add, list, and delete image", func(t *testing.T) {
		setup(t)
		p := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		altText := "Test alt"
		img := &product.Image{
			ProductID: p.ID,
			URL:       "https://example.com/image.jpg",
			AltText:   &altText,
			SortOrder: 0,
		}

		err := repo.AddImage(ctx, img)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, img.ID)

		images, err := repo.GetImagesByProductID(ctx, p.ID)
		require.NoError(t, err)
		require.Len(t, images, 1)
		assert.Equal(t, img.ID, images[0].ID)
		assert.Equal(t, img.URL, images[0].URL)

		err = repo.DeleteImage(ctx, img.ID)
		require.NoError(t, err)

		images, err = repo.GetImagesByProductID(ctx, p.ID)
		require.NoError(t, err)
		assert.Empty(t, images)

		err = repo.DeleteImage(ctx, img.ID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
