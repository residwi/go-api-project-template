package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopUploader_Upload(t *testing.T) {
	uploader := &NoopUploader{}
	url, err := uploader.Upload(context.Background(), "photo.jpg", strings.NewReader("data"), "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, "/uploads/photo.jpg", url)
}
