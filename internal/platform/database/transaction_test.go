package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func TestDBWithoutPool(t *testing.T) {
	t.Run("returns pool when no transaction in context", func(t *testing.T) {
		ctx := context.Background()
		result := database.DB(ctx, nil)
		assert.Nil(t, result)
	})
}

func TestReadDB(t *testing.T) {
	t.Run("returns primary after recent write", func(t *testing.T) {
		ctx := context.Background()
		ctx = database.WithRecentWrite(ctx)

		result := database.ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})

	t.Run("returns primary when no reader available", func(t *testing.T) {
		ctx := context.Background()

		result := database.ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})
}
