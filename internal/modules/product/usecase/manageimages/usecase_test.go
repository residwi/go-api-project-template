package manageimages

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
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
)

func TestCommand_Add(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

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
		img, err := cmd.Add(context.Background(), productID, Params{
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
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := cmd.Add(context.Background(), uuid.New(), Params{
			URL: "https://img.example.com/x.jpg",
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("add image repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&domain.Product{ID: productID}, nil)
		repo.EXPECT().AddImage(mock.Anything, mock.Anything).Return(errors.New("db error"))

		_, err := cmd.Add(context.Background(), productID, Params{
			URL: "https://img.example.com/x.jpg",
		})
		require.Error(t, err)
	})
}

func TestCommand_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		imageID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, productID).
			Return(&domain.Product{ID: productID}, nil)
		repo.EXPECT().DeleteImage(mock.Anything, imageID).Return(nil)

		err := cmd.Delete(context.Background(), productID, imageID)
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		err := cmd.Delete(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
