package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetup(t *testing.T) {
	t.Parallel()

	t.Run("json is the default format", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		setup(&buf, "info", "json").Info("hello")

		assert.Equal(t, map[string]any{"level": "INFO", "msg": "hello"}, decodeRecord(t, &buf))
	})

	t.Run("an unknown format falls back to json", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		setup(&buf, "info", "unknown").Info("hello")

		// decodeRecord fails the test if the output is not JSON, which is the assertion.
		assert.Equal(t, map[string]any{"level": "INFO", "msg": "hello"}, decodeRecord(t, &buf))
	})

	t.Run("text format writes key=value", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		setup(&buf, "info", "text").Info("hello")

		assert.Contains(t, buf.String(), `level=INFO msg=hello`)
	})

	t.Run("debug level lets debug records through", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		setup(&buf, "debug", "json").Debug("hello")

		assert.Contains(t, buf.String(), `"msg":"hello"`)
	})

	t.Run("an unknown level stays at info, so debug is dropped", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		log := setup(&buf, "unknown", "json")
		log.Debug("dropped")

		assert.Empty(t, buf.String())
	})

	t.Run("warning is accepted as an alias for warn", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		log := setup(&buf, "warning", "json")
		log.Info("dropped")
		log.Warn("kept")

		assert.NotContains(t, buf.String(), "dropped")
		assert.Contains(t, buf.String(), `"msg":"kept"`)
	})

	t.Run("error level drops warnings", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		log := setup(&buf, "error", "json")
		log.Warn("dropped")

		assert.Empty(t, buf.String())
	})

	t.Run("context attributes reach records through Setup", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))

		setup(&buf, "info", "json").InfoContext(ctx, "hello")

		assert.Contains(t, buf.String(), `"request_id":"req-1"`)
	})
}
