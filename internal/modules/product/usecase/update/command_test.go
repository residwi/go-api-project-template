package update

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

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
		p, err := cmd.Execute(context.Background(), id, Params{
			Name: &newName,
		})
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
		cmd := New(repo)

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
		result, err := cmd.Execute(context.Background(), id, Params{
			CategoryID:     &catID,
			Name:           &newName,
			Description:    &newDesc,
			Price:          &newPrice,
			CompareAtPrice: &newCompare,
			SKU:            &newSKU,
			Status:         &newStatus,
		})

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
		cmd := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := cmd.Execute(context.Background(), uuid.New(), Params{})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.Product{
				ID:     id,
				Name:   "Old",
				Slug:   "old",
				Price:  money.New(1000, "USD"),
				Status: domain.StatusDraft,
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		newName := "New"
		p, err := cmd.Execute(context.Background(), id, Params{
			Name: &newName,
		})
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestCommand_Execute_RepricesStoredCompareAtPriceIntoTheNewCurrency(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	cmd := New(repo)

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
	result, err := cmd.Execute(context.Background(), id, Params{Price: &newPrice})
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
