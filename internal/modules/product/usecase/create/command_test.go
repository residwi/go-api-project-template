package create

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

	t.Run("success sets slug default currency and status", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reg := NewMockInventoryRegistrar(t)
		cmd := New(repo, reg)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Product")).
			Run(func(_ context.Context, p *domain.Product) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)
		reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := cmd.Execute(context.Background(), Params{
			Name:  "Cool Widget",
			Price: money.New(1999, ""),
		})
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
		reg := NewMockInventoryRegistrar(t)
		cmd := New(repo, reg)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p *domain.Product) bool {
			return p.Price.Currency == "EUR" && p.Status == domain.StatusPublished
		})).Run(func(_ context.Context, p *domain.Product) {
			p.ID = uuid.New()
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
		}).Return(nil)
		reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)

		result, err := cmd.Execute(context.Background(), Params{
			Name:   "Widget",
			Price:  money.New(1000, "EUR"),
			Status: domain.StatusPublished,
		})
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
		reg := NewMockInventoryRegistrar(t)
		cmd := New(repo, reg)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		p, err := cmd.Execute(context.Background(), Params{
			Name:  "Widget",
			Price: money.New(1000, ""),
		})
		assert.Nil(t, p)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestCommand_Execute_RegistersZeroInventoryLevel(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	reg := NewMockInventoryRegistrar(t)
	cmd := New(repo, reg)

	repo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, p *domain.Product) error {
			p.ID = uuid.New()
			return nil
		})
	reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil)
	// No GetAvailability expectation -- create depends only on InventoryRegistrar,
	// so mockery fails the test if Execute reads inventory back for a value it
	// already knows by construction.

	p, err := cmd.Execute(context.Background(), Params{
		Name:  "Widget",
		Price: money.New(1500, ""),
	})
	require.NoError(t, err)
	require.NotNil(t, p)
}
