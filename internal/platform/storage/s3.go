package storage

import (
	"context"
	"fmt"
	"io"
)

type Uploader interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) (url string, err error)
}

type NoopUploader struct{}

func (n *NoopUploader) Upload(_ context.Context, key string, _ io.Reader, _ string) (string, error) {
	return fmt.Sprintf("/uploads/%s", key), nil
}
