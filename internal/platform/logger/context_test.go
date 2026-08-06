package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextHandler(t *testing.T) {
	t.Parallel()

	t.Run("attributes stashed in the context reach the record", func(t *testing.T) {
		t.Parallel()

		log, buf := newBufferLogger()
		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))

		log.InfoContext(ctx, "hello")

		assert.Equal(t, map[string]any{
			"level":      "INFO",
			"msg":        "hello",
			"request_id": "req-1",
		}, decodeRecord(t, buf))
	})

	t.Run("nested calls accumulate instead of replacing", func(t *testing.T) {
		t.Parallel()

		log, buf := newBufferLogger()
		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))
		ctx = WithAttrs(ctx, slog.String("user_id", "user-1"))

		log.InfoContext(ctx, "hello")

		assert.Equal(t, map[string]any{
			"level":      "INFO",
			"msg":        "hello",
			"request_id": "req-1",
			"user_id":    "user-1",
		}, decodeRecord(t, buf))
	})

	t.Run("an empty context adds nothing", func(t *testing.T) {
		t.Parallel()

		log, buf := newBufferLogger()

		log.InfoContext(context.Background(), "hello")

		assert.Equal(t, map[string]any{"level": "INFO", "msg": "hello"}, decodeRecord(t, buf))
	})

	t.Run("With keeps the wrapper, so context attributes survive it", func(t *testing.T) {
		t.Parallel()

		log, buf := newBufferLogger()
		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))

		log.With(slog.String("layer", "service")).InfoContext(ctx, "hello")

		assert.Equal(t, map[string]any{
			"level":      "INFO",
			"msg":        "hello",
			"layer":      "service",
			"request_id": "req-1",
		}, decodeRecord(t, buf))
	})

	t.Run("WithGroup keeps the wrapper, so context attributes survive it", func(t *testing.T) {
		t.Parallel()

		log, buf := newBufferLogger()
		grouped := slog.New(log.Handler().WithGroup("scope"))
		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))

		grouped.InfoContext(ctx, "hello")

		assert.Equal(t, map[string]any{
			"level": "INFO",
			"msg":   "hello",
			"scope": map[string]any{"request_id": "req-1"},
		}, decodeRecord(t, buf))
	})

	t.Run("sibling contexts do not overwrite each other", func(t *testing.T) {
		t.Parallel()

		// Three levels are needed to reproduce the stomp: append grows the slice to
		// capacity 4 while its length is 3, leaving one slot two children both claim.
		parent := WithAttrs(context.Background(), slog.String("a", "1"))
		parent = WithAttrs(parent, slog.String("b", "2"))
		parent = WithAttrs(parent, slog.String("c", "3"))
		left := WithAttrs(parent, slog.String("branch", "left"))
		right := WithAttrs(parent, slog.String("branch", "right"))

		leftLog, leftBuf := newBufferLogger()
		leftLog.InfoContext(left, "hello")
		rightLog, rightBuf := newBufferLogger()
		rightLog.InfoContext(right, "hello")

		assert.Equal(t, "left", decodeRecord(t, leftBuf)["branch"])
		assert.Equal(t, "right", decodeRecord(t, rightBuf)["branch"])
	})

	t.Run("two handlers given the same record do not corrupt it", func(t *testing.T) {
		t.Parallel()

		// Eight attributes are the threshold: five fill the record's inline array and
		// the rest grow its overflow slice to length 3, capacity 4 -- one spare slot
		// that both handlers would claim. slog detects the collision and appends a
		// "!BUG" attribute rather than failing, so a corrupt record still logs.
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
		for i := range 8 {
			record.AddAttrs(slog.Int(fmt.Sprintf("a%d", i), i))
		}

		ctx := WithAttrs(context.Background(), slog.String("request_id", "req-1"))
		var leftBuf, rightBuf bytes.Buffer
		left := ContextHandler{Handler: slog.NewJSONHandler(&leftBuf, nil)}
		right := ContextHandler{Handler: slog.NewJSONHandler(&rightBuf, nil)}

		require.NoError(t, left.Handle(ctx, record))
		require.NoError(t, right.Handle(ctx, record))

		assert.NotContains(t, leftBuf.String(), "!BUG")
		assert.NotContains(t, rightBuf.String(), "!BUG")
		assert.Equal(t, "req-1", decodeRecord(t, &rightBuf)["request_id"])
	})
}

func newBufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(ContextHandler{Handler: slog.NewJSONHandler(&buf, nil)}), &buf
}

// decodeRecord drops the timestamp so callers can compare the whole map.
func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	delete(record, "time")

	return record
}
