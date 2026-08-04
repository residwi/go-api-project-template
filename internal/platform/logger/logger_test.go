package logger

import (
	"testing"
)

func TestSetup(t *testing.T) {
	t.Parallel()

	t.Run("json format with info level", func(t *testing.T) {
		t.Parallel()

		Setup("info", "json")
	})

	t.Run("text format with debug level", func(t *testing.T) {
		t.Parallel()

		Setup("debug", "text")
	})

	t.Run("warn level", func(t *testing.T) {
		t.Parallel()

		Setup("warn", "json")
	})

	t.Run("warning level alias", func(t *testing.T) {
		t.Parallel()

		Setup("warning", "json")
	})

	t.Run("error level", func(t *testing.T) {
		t.Parallel()

		Setup("error", "json")
	})

	t.Run("unknown level defaults to info", func(t *testing.T) {
		t.Parallel()

		Setup("unknown", "json")
	})

	t.Run("unknown format defaults to json", func(t *testing.T) {
		t.Parallel()

		Setup("info", "unknown")
	})
}
