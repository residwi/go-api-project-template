package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_product")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns product", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Name, got.Name)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	// Both handlers serialise CompareAtPrice as *int64 with omitempty, so a
	// scan that turned a NULL column into a pointer-to-zero would publish
	// "compare_at_price": 0 on every product that has no compare-at price --
	// omitempty does not suppress a non-nil pointer.
	t.Run("leaves a NULL compare_at_price nil rather than a denominated zero", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Nil(t, got.CompareAtPrice)
	})

	t.Run("reads a set compare_at_price back at its stored amount", func(t *testing.T) {
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, compare_at_price, currency)
			 VALUES ($1, 'Discounted', $2, 'desc', 1000, 2999, 'USD')`,
			id, "slug-"+id.String(),
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
		})

		repo := New(database.DB{Primary: testPool})
		got, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, got.CompareAtPrice)
		assert.Equal(t, money.New(2999, "USD"), *got.CompareAtPrice)
	})
}

func TestPostgresRepository_GetBySlug(t *testing.T) {
	t.Run("returns product by slug", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetBySlug(context.Background(), p.Slug)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetBySlug(context.Background(), "nonexistent-slug")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_ListPublished(t *testing.T) {
	t.Run("returns published products", func(t *testing.T) {
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

		repo := New(database.DB{Primary: testPool})
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Limit: 10,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products)
		for _, p := range products {
			assert.Equal(t, domain.StatusPublished, p.Status)
		}
	})

	t.Run("filters by category ID", func(t *testing.T) {
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

		repo := New(database.DB{Primary: testPool})
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
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'UniqueSearchable Widget', $2, 'desc', 1000, 'USD', 'published')`,
			id, "search-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(database.DB{Primary: testPool})
		products, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Search: "UniqueSearchable",
			Limit:  10,
		})
		require.NoError(t, err)
		require.NotEmpty(t, products)
		assert.Equal(t, id, products[0].ID)
	})

	t.Run("filters by min and max price", func(t *testing.T) {
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Price Filter Product', $2, 'desc', 5000, 'USD', 'published')`,
			id, "price-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(database.DB{Primary: testPool})
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
			// MinPrice/MaxPrice stay bare int64: a Money would claim a denomination the
			// query params never carry.
			assert.GreaterOrEqual(t, p.Price.Amount, minPrice)
			assert.LessOrEqual(t, p.Price.Amount, maxPrice)
		}
	})

	t.Run("cursor pagination", func(t *testing.T) {
		for range 3 {
			id := uuid.New()
			_, err := testPool.Exec(context.Background(),
				`INSERT INTO products (id, name, slug, description, price, currency, status)
				 VALUES ($1, $2, $3, 'desc', 1000, 'USD', 'published')`,
				id, "cursor-prod-"+id.String()[:8], "cursor-"+id.String()[:8])
			require.NoError(t, err)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
		}

		repo := New(database.DB{Primary: testPool})

		products, nextCursor, hasMore, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Limit: 2,
		})
		require.NoError(t, err)
		assert.True(t, hasMore)
		assert.NotEmpty(t, nextCursor)
		assert.Len(t, products, 2)

		products2, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Cursor: nextCursor,
			Limit:  2,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, products2)
		for _, p2 := range products2 {
			for _, p1 := range products {
				assert.NotEqual(t, p1.ID, p2.ID)
			}
		}
	})
}

func TestPostgresRepository_ListPublished_InvalidCursor(t *testing.T) {
	t.Run("returns bad request error on invalid cursor", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		_, _, _, err := repo.ListPublished(context.Background(), product.PublishedListParams{
			Cursor: "invalid-cursor-value",
			Limit:  10,
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns all products with total", func(t *testing.T) {
		seedProduct(t)
		seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		assert.NotEmpty(t, products)
	})

	t.Run("filters by status", func(t *testing.T) {
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Draft Product', $2, 'desc', 1000, 'USD', 'draft')`,
			id, "draft-"+id.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(database.DB{Primary: testPool})
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			Status:     "draft",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range products {
			assert.Equal(t, domain.StatusDraft, p.Status)
		}
	})

	t.Run("filters by category ID", func(t *testing.T) {
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

		repo := New(database.DB{Primary: testPool})
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			CategoryID: &catID,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range products {
			assert.Equal(t, &catID, p.CategoryID)
		}
	})

	t.Run("filters by search on name, description, and sku", func(t *testing.T) {
		sku := "UNIQSKU-" + uuid.New().String()[:8]
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, sku)
			 VALUES ($1, 'Admin Search Product', $2, 'desc', 1000, 'USD', 'draft', $3)`,
			id, "admin-search-"+id.String()[:8], sku)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(database.DB{Primary: testPool})
		products, total, err := repo.ListAdmin(context.Background(), product.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			Search:     sku,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, products)
		assert.Equal(t, id, products[0].ID)
	})
}

func TestPostgresRepository_GetByIDsIncludingDeleted(t *testing.T) {
	t.Run("returns soft-deleted and unpublished products alongside live ones", func(t *testing.T) {
		live := seedProduct(t)

		archivedID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status)
			 VALUES ($1, 'Archived', $2, 'desc', 1000, 'USD', 'archived')`,
			archivedID, "archived-"+archivedID.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, archivedID) })

		// status is left 'published': Service.Delete only sets deleted_at, so this
		// mirrors a real withdrawn-but-still-published row.
		deletedID := uuid.New()
		_, err = testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, currency, status, deleted_at)
			 VALUES ($1, 'Deleted', $2, 'desc', 1000, 'USD', 'published', NOW())`,
			deletedID, "deleted-"+deletedID.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, deletedID) })

		repo := New(database.DB{Primary: testPool})
		got, err := repo.GetByIDsIncludingDeleted(
			context.Background(),
			[]uuid.UUID{live.ID, archivedID, deletedID, uuid.New()},
		)
		require.NoError(t, err)

		byID := make(map[uuid.UUID]domain.Product, len(got))
		for _, p := range got {
			byID[p.ID] = p
		}
		require.Len(t, byID, 3, "the fourth id has no row at all and must simply be absent")
		assert.Equal(t, live.Status, byID[live.ID].Status)
		assert.Nil(t, byID[live.ID].DeletedAt)
		assert.Equal(t, "archived", byID[archivedID].Status)
		assert.Nil(t, byID[archivedID].DeletedAt)
		assert.Contains(t, byID, deletedID, "a soft-deleted product must still be returned")
		// Service.Delete changes only deleted_at, so a caller must read DeletedAt, not
		// Status, to know this product is unsellable.
		assert.Equal(t, "published", byID[deletedID].Status)
		require.NotNil(
			t,
			byID[deletedID].DeletedAt,
			"a soft-deleted product must carry DeletedAt so a consumer can flag it unsellable",
		)
	})

	t.Run("returns empty slice for empty ids", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByIDsIncludingDeleted(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPostgresRepository_CountPublishedByCategory(t *testing.T) {
	t.Run("returns zero when no products", func(t *testing.T) {
		catID := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Empty Cat', $2, true)`,
			catID, "empty-cat-"+catID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, catID) })

		repo := New(database.DB{Primary: testPool})
		count, err := repo.CountPublishedByCategory(context.Background(), catID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns count of published products", func(t *testing.T) {
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

		repo := New(database.DB{Primary: testPool})
		count, err := repo.CountPublishedByCategory(ctx, catID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates product", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		desc := "A description"
		p := &domain.Product{
			Name:        "New Product",
			Slug:        "new-product-" + uuid.New().String(),
			Description: &desc,
			Price:       money.New(1000, "USD"),
			Status:      domain.StatusPublished,
		}

		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.False(t, p.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, p.ID)
		})
	})

	// Create writes price and compare_at_price as separate params under the
	// price's own currency (never independently), so this checks the row landed
	// with the right amounts and currency, not the read-side scanner -- that nil
	// branch is exercised in TestPostgresRepository_Update's own compare-at-price
	// subtest, against this same GetByID.
	t.Run("round-trips both amounts under the row's single currency", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		compareAt := money.New(2999, "EUR")
		p := &domain.Product{
			Name:           "Denominated Product",
			Slug:           "denominated-" + uuid.New().String(),
			Price:          money.New(1999, "EUR"),
			CompareAtPrice: &compareAt,
			Status:         domain.StatusPublished,
		}
		require.NoError(t, repo.Create(context.Background(), p))
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, p.ID)
		})

		var gotPrice, gotCompareAt int64
		var gotCurrency string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT price, compare_at_price, currency FROM products WHERE id = $1`, p.ID,
		).Scan(&gotPrice, &gotCompareAt, &gotCurrency))
		assert.Equal(t, int64(1999), gotPrice)
		assert.Equal(t, int64(2999), gotCompareAt)
		assert.Equal(t, "EUR", gotCurrency)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		existing := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		dup := &domain.Product{
			Name:   "Duplicate",
			Slug:   existing.Slug,
			Price:  money.New(500, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates product fields", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		p.Name = "Updated Product"
		p.Price = money.New(2000, "EUR")
		p.Status = domain.StatusArchived
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Product", got.Name)
		// The whole Money: asserting only the amount would pass even if Update wrote
		// the price under the wrong denomination.
		assert.Equal(t, money.New(2000, "EUR"), got.Price)
		assert.Equal(t, domain.StatusArchived, got.Status)
	})

	// Nothing else in this package writes a compare-at price then clears it, so
	// this is the only test that exercises the scanner's nil branch -- where an
	// absent price coming back as a denominated zero would publish
	// `compare_at_price: 0`.
	t.Run("nulling compare_at_price round-trips as nil, not a denominated zero", func(t *testing.T) {
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, compare_at_price, currency, status)
			 VALUES ($1, 'Denominated Product', $2, 'desc', 1999, 2999, 'EUR', 'published')`,
			id, "denominated-"+id.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(database.DB{Primary: testPool})
		p, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, p.CompareAtPrice)
		assert.Equal(t, money.New(2999, "EUR"), *p.CompareAtPrice,
			"compare_at_price has no currency column of its own, so it must read back denominated from the row's")

		p.CompareAtPrice = nil
		require.NoError(t, repo.Update(context.Background(), p))

		got, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Nil(t, got.CompareAtPrice)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		p := &domain.Product{
			ID:     uuid.New(),
			Name:   "Ghost",
			Slug:   "ghost-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Update(context.Background(), p)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		p1 := seedProduct(t)
		p2 := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		p2.Slug = p1.Slug
		err := repo.Update(context.Background(), p2)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

// TestSearchString exercises isUniqueViolation returning false: an update that
// leaves the slug unchanged hits no unique constraint.
func TestSearchString(t *testing.T) {
	t.Run("returns false when substring not found", func(t *testing.T) {
		existing := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		existing.Name = "Updated Name"
		err := repo.Update(context.Background(), existing)
		require.NoError(t, err)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes product", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		err := repo.Delete(context.Background(), p.ID)
		require.NoError(t, err)

		var deletedAt *string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT deleted_at::text FROM products WHERE id = $1`, p.ID).Scan(&deletedAt))
		require.NotNil(t, deletedAt, "Delete must set deleted_at")
	})

	t.Run("returns not found after delete", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, p.ID))

		err := repo.Delete(ctx, p.ID)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("returns not found for nonexistent product", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_Images(t *testing.T) {
	t.Run("add, list, and delete image", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		altText := "Test alt"
		img := &domain.Image{
			ProductID: p.ID,
			URL:       "https://example.com/image.jpg",
			AltText:   &altText,
			SortOrder: 0,
		}

		err := repo.AddImage(ctx, img)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, img.ID)

		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM product_images WHERE id = $1 AND url = $2`, img.ID, img.URL,
		).Scan(&count))
		assert.Equal(t, 1, count)

		err = repo.DeleteImage(ctx, img.ID)
		require.NoError(t, err)

		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM product_images WHERE id = $1`, img.ID,
		).Scan(&count))
		assert.Equal(t, 0, count)

		err = repo.DeleteImage(ctx, img.ID)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(database.DB{Primary: testPool})

	t.Run("GetByID", func(t *testing.T) {
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetBySlug", func(t *testing.T) {
		_, err := repo.GetBySlug(cancelledCtx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("ListPublished", func(t *testing.T) {
		_, _, _, err := repo.ListPublished(cancelledCtx, product.PublishedListParams{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		_, _, err := repo.ListAdmin(
			cancelledCtx,
			product.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}},
		)
		assert.Error(t, err)
	})

	t.Run("GetByIDsIncludingDeleted", func(t *testing.T) {
		_, err := repo.GetByIDsIncludingDeleted(cancelledCtx, []uuid.UUID{uuid.New()})
		assert.Error(t, err)
	})

	t.Run("GetImagesByProductID", func(t *testing.T) {
		_, err := repo.GetImagesByProductID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("CountPublishedByCategory", func(t *testing.T) {
		_, err := repo.CountPublishedByCategory(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		p := &domain.Product{
			Name:   "X",
			Slug:   "x-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		p := &domain.Product{
			ID:     uuid.New(),
			Name:   "X",
			Slug:   "x-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Update(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("AddImage", func(t *testing.T) {
		img := &domain.Image{ProductID: uuid.New(), URL: "https://example.com/img.jpg", SortOrder: 0}
		err := repo.AddImage(cancelledCtx, img)
		assert.Error(t, err)
	})

	t.Run("DeleteImage", func(t *testing.T) {
		err := repo.DeleteImage(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

// seedProduct inserts a product with a fresh id and cleans it up on test
// completion -- this package never truncates the table.
func seedProduct(t *testing.T) *domain.Product {
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

	repo := New(database.DB{Primary: testPool})
	p, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return p
}
