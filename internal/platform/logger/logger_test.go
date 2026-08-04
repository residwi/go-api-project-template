package logger

import (
	"testing"
)

func TestSetup(t *testing.T) {
	t.Run("json format with info level", func(_ *testing.T) {
		Setup("info", "json")
	})

	t.Run("text format with debug level", func(_ *testing.T) {
		Setup("debug", "text")
	})

	t.Run("warn level", func(_ *testing.T) {
		Setup("warn", "json")
	})

	t.Run("warning level alias", func(_ *testing.T) {
		Setup("warning", "json")
	})

	t.Run("error level", func(_ *testing.T) {
		Setup("error", "json")
	})

	t.Run("unknown level defaults to info", func(_ *testing.T) {
		Setup("unknown", "json")
	})

	t.Run("unknown format defaults to json", func(_ *testing.T) {
		Setup("info", "unknown")
	})
}
