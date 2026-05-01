package category_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/category"
)

func newTestService(t *testing.T) *category.Service {
	t.Helper()
	repo := category.NewPostgresRepository(testPool)
	return category.NewService(repo, testPool)
}

func createCategory(t *testing.T, svc *category.Service, parentID *uuid.UUID) *category.Category {
	t.Helper()
	cat, err := svc.Create(context.Background(), category.CreateCategoryRequest{
		Name:     "Cat-" + uuid.New().String()[:8],
		ParentID: parentID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID)
	})
	return cat
}

func TestServiceCreate_ValidateParent(t *testing.T) {
	t.Run("creates child with valid parent", func(t *testing.T) {
		svc := newTestService(t)
		parent := createCategory(t, svc, nil)

		child, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name:     "Child-" + uuid.New().String()[:8],
			ParentID: &parent.ID,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, child.ID)
		})

		assert.Equal(t, &parent.ID, child.ParentID)
	})

	t.Run("returns error for non-existent parent", func(t *testing.T) {
		svc := newTestService(t)
		nonExistent := uuid.New()

		_, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name:     "Orphan-" + uuid.New().String()[:8],
			ParentID: &nonExistent,
		})
		require.ErrorIs(t, err, core.ErrBadRequest)
		assert.ErrorContains(t, err, "parent category not found")
	})

	t.Run("returns error when category is its own parent", func(t *testing.T) {
		svc := newTestService(t)
		cat := createCategory(t, svc, nil)

		_, err := svc.Update(context.Background(), cat.ID, category.UpdateCategoryRequest{
			ParentID: &cat.ID,
		})
		require.ErrorIs(t, err, core.ErrBadRequest)
		assert.ErrorContains(t, err, "cannot be its own parent")
	})

	t.Run("returns error when depth exceeds maximum of 5", func(t *testing.T) {
		svc := newTestService(t)

		// Build a chain: level1 → level2 → level3 → level4 → level5
		level1 := createCategory(t, svc, nil)
		level2 := createCategory(t, svc, &level1.ID)
		level3 := createCategory(t, svc, &level2.ID)
		level4 := createCategory(t, svc, &level3.ID)
		level5 := createCategory(t, svc, &level4.ID)

		// Adding a 6th level should fail
		_, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name:     "Level6-" + uuid.New().String()[:8],
			ParentID: &level5.ID,
		})
		require.ErrorIs(t, err, core.ErrBadRequest)
		assert.ErrorContains(t, err, "depth exceeds maximum of 5")
	})

	t.Run("returns error on circular parent reference", func(t *testing.T) {
		svc := newTestService(t)

		// Build A → B → C
		catA := createCategory(t, svc, nil)
		catB := createCategory(t, svc, &catA.ID)
		catC := createCategory(t, svc, &catB.ID)

		// Try to set A's parent to C → circular
		_, err := svc.Update(context.Background(), catA.ID, category.UpdateCategoryRequest{
			ParentID: &catC.ID,
		})
		require.ErrorIs(t, err, core.ErrBadRequest)
		assert.ErrorContains(t, err, "circular parent reference")
	})
}

func TestServiceCreate_ValidateParent_DBError(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		svc := newTestService(t)
		parentID := uuid.New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := svc.Create(ctx, category.CreateCategoryRequest{
			Name:     "Test-" + uuid.New().String()[:8],
			ParentID: &parentID,
		})
		assert.Error(t, err)
	})
}

func TestServiceUpdate_MoveParent(t *testing.T) {
	t.Run("moves category to a new parent", func(t *testing.T) {
		svc := newTestService(t)

		oldParent := createCategory(t, svc, nil)
		newParent := createCategory(t, svc, nil)
		child := createCategory(t, svc, &oldParent.ID)

		updated, err := svc.Update(context.Background(), child.ID, category.UpdateCategoryRequest{
			ParentID: &newParent.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, &newParent.ID, updated.ParentID)
	})
}
